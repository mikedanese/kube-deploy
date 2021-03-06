package awstasks

import (
	"fmt"

	"bytes"
	"crypto/md5"
	"encoding/base64"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"golang.org/x/crypto/ssh"
	"k8s.io/kube-deploy/upup/pkg/fi"
	"k8s.io/kube-deploy/upup/pkg/fi/cloudup/awsup"
	"strings"
)

type SSHKey struct {
	Name      *string
	PublicKey fi.Resource

	KeyFingerprint *string
}

var _ fi.CompareWithID = &SecurityGroup{}

func (e *SSHKey) CompareWithID() *string {
	return e.Name
}

func (e *SSHKey) String() string {
	return fi.TaskAsString(e)
}
func (e *SSHKey) Find(c *fi.Context) (*SSHKey, error) {
	cloud := c.Cloud.(*awsup.AWSCloud)

	request := &ec2.DescribeKeyPairsInput{
		KeyNames: []*string{e.Name},
	}

	response, err := cloud.EC2.DescribeKeyPairs(request)
	if awsErr, ok := err.(awserr.Error); ok {
		if awsErr.Code() == "InvalidKeyPair.NotFound" {
			return nil, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error listing SSHKeys: %v", err)
	}

	if response == nil || len(response.KeyPairs) == 0 {
		return nil, nil
	}

	if len(response.KeyPairs) != 1 {
		return nil, fmt.Errorf("Found multiple SSHKeys with Name %q", *e.Name)
	}

	k := response.KeyPairs[0]

	actual := &SSHKey{
		Name:           k.KeyName,
		KeyFingerprint: k.KeyFingerprint,
	}

	return actual, nil
}

func computeAwsKeyFingerprint(publicKey fi.Resource) (string, error) {
	publicKeyString, err := fi.ResourceAsString(publicKey)
	if err != nil {
		return "", fmt.Errorf("error reading SSH public key: %v", err)
	}

	tokens := strings.Split(publicKeyString, " ")
	if len(tokens) < 2 {
		return "", fmt.Errorf("error parsing SSH public key: %s", publicKeyString)
	}

	sshPublicKeyBytes, err := base64.StdEncoding.DecodeString(tokens[1])
	if len(tokens) < 2 {
		return "", fmt.Errorf("error decoding SSH public key: %s", publicKeyString)
	}

	// We don't technically need to parse and remarshal it, but it ensures the key is valid
	sshPublicKey, err := ssh.ParsePublicKey(sshPublicKeyBytes)
	if err != nil {
		return "", fmt.Errorf("error parsing SSH public key: %v", err)
	}

	h := md5.Sum(sshPublicKey.Marshal())
	sshKeyFingerprint := fmt.Sprintf("%x", h)

	var colonSeparated bytes.Buffer
	for i := 0; i < len(sshKeyFingerprint); i++ {
		if (i%2) == 0 && i != 0 {
			colonSeparated.WriteByte(':')
		}
		colonSeparated.WriteByte(sshKeyFingerprint[i])
	}

	return colonSeparated.String(), nil
}

func (e *SSHKey) Run(c *fi.Context) error {
	if e.KeyFingerprint == nil && e.PublicKey != nil {
		keyFingerprint, err := computeAwsKeyFingerprint(e.PublicKey)
		if err != nil {
			return fmt.Errorf("error computing key fingerpring for SSH key: %v", err)
		}
		glog.V(2).Infof("Computed SSH key fingerprint as %q", keyFingerprint)
		e.KeyFingerprint = &keyFingerprint
	}
	return fi.DefaultDeltaRunMethod(e, c)
}

func (s *SSHKey) CheckChanges(a, e, changes *SSHKey) error {
	if a != nil {
		if changes.Name != nil {
			return fi.CannotChangeField("Name")
		}

		if changes.KeyFingerprint == nil {
			if e.PublicKey != nil && a.PublicKey == nil {
				glog.V(2).Infof("SSH key fingerprints match; assuming public keys match")
				changes.PublicKey = nil
			}
		} else {
			glog.V(2).Infof("Computed SSH key fingerprint mismatch: %q %q", fi.StringValue(e.KeyFingerprint), fi.StringValue(a.KeyFingerprint))
		}
	}
	return nil
}

func (_ *SSHKey) RenderAWS(t *awsup.AWSAPITarget, a, e, changes *SSHKey) error {
	if a == nil {
		glog.V(2).Infof("Creating SSHKey with Name:%q", *e.Name)

		request := &ec2.ImportKeyPairInput{
			KeyName: e.Name,
		}

		if e.PublicKey != nil {
			d, err := fi.ResourceAsBytes(e.PublicKey)
			if err != nil {
				return fmt.Errorf("error rendering SSHKey PublicKey: %v", err)
			}
			request.PublicKeyMaterial = d
		}

		response, err := t.Cloud.EC2.ImportKeyPair(request)
		if err != nil {
			return fmt.Errorf("error creating SSHKey: %v", err)
		}

		e.KeyFingerprint = response.KeyFingerprint
	}

	return nil //return output.AddAWSTags(cloud.Tags(), v, "vpc")
}
