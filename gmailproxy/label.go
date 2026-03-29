package gmailproxy

import (
	"fmt"

	"google.golang.org/api/gmail/v1"
)

// ResolveLabelID finds the Gmail label ID for a given label name.
func ResolveLabelID(svc *gmail.Service, labelName string) (string, error) {
	resp, err := svc.Users.Labels.List("me").Do()
	if err != nil {
		return "", fmt.Errorf("listing labels: %w", err)
	}

	for _, l := range resp.Labels {
		if l.Name == labelName {
			return l.Id, nil
		}
	}
	return "", fmt.Errorf("label %q not found", labelName)
}

// HasLabel checks if a message has the given label ID.
func HasLabel(msg *gmail.Message, labelID string) bool {
	for _, id := range msg.LabelIds {
		if id == labelID {
			return true
		}
	}
	return false
}
