package server

import (
	"fmt"
	"strings"
)

func parseJobID(jobID string) (namespace, name string, err error) {
	items := strings.Split(jobID, ".")
	if len(items) != 2 {
		return "", "", fmt.Errorf("job id %s must be in namespace.name format", jobID)
	}
	return items[0], items[1], nil
}
