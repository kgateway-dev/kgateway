package azure

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/basgys/goxml2json"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

var missingAnnotationErr = errors.Errorf("Azure Function Discovery requires that a secret ref "+
	"for a secret containing the the Publish Profile for the Function App "+
	"to be specified in the annotations for each Azure Function Upstream. "+
	"The annotation key is %v. The annotations should contain the annotation %v: [your_secret_ref]",
	annotationKey, annotationKey)

const (
	annotationKey = "gloo.solo.io/azure_publish_profile"

	publishProfileKey = "publish_profile"
)

type publishProfile struct {
	PublishData struct {
		PublishProfile []struct {
			ProfileName       string `json:"-profileName"`
			PublishMethod     string `json:"-publishMethod"`
			PublishURL        string `json:"-publishUrl"`
			MsdeploySite      string `json:"-msdeploySite,omitempty"`
			UserName          string `json:"-userName"`
			UserPWD           string `json:"-userPWD"`
			DestinationAppURL string `json:"-destinationAppUrl"`
			ControlPanelLink  string `json:"-controlPanelLink"`
			WebSystem         string `json:"-webSystem"`
			FtpPassiveMode    string `json:"-ftpPassiveMode,omitempty"`
		} `json:"publishProfile"`
	} `json:"publishData"`
}

func getUserPassword(us *v1.Upstream, secrets secretwatcher.SecretMap) (string, error) {
	ref, err := getSecretRef(us)
	if err != nil {
		return "", err
	}
	publishProfileSecret, ok := secrets[ref]
	if !ok {
		return "", errors.Errorf("secret ref %s not found", ref)
	}
	publishProfileXML, ok := publishProfileSecret.Data[publishProfileKey]
	if !ok {
		return "", errors.Errorf("key %v missing from provided secret", publishProfileKey)
	}
	if !utf8.Valid([]byte(publishProfileXML)) {
		return "", errors.Errorf("contents of %s not a valid string", publishProfileKey)
	}
	jsn, err := xml2json.Convert(strings.NewReader(publishProfileXML))
	if err != nil {
		return "", errors.Wrap(err, "parsing publish profile xml")
	}
	var profile publishProfile
	if err := json.Unmarshal(jsn.Bytes(), &profile); err != nil {
		return "", errors.Wrap(err, "parsing publish profile json")
	}
	if len(profile.PublishData.PublishProfile) < 1 {
		return "", errors.Errorf("publish profile contained no profiles")
	}

	return profile.PublishData.PublishProfile[0].UserPWD, nil
}

func getSecretRef(us *v1.Upstream) (string, error) {
	if us.Metadata == nil {
		return "", missingAnnotationErr
	}
	secretRef, ok := us.Metadata.Annotations[annotationKey]
	if !ok {
		return "", missingAnnotationErr
	}
	return secretRef, nil
}
