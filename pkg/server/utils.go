package server

import (
	"fmt"
	"strconv"
	"strings"

	commontypes "opendilab.org/di-orchestrator/pkg/common/types"
)

func parseJobID(jobID string) (namespace, name string, generation int, err error) {
	items := strings.Split(jobID, ".")
	if len(items) != 3 {
		return "", "", -1, &commontypes.DIError{
			Type:    commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("job id %s must be in namespace.name.generation format", jobID)}
	}
	gen, err := strconv.Atoi(items[2])
	if err != nil {
		return "", "", -1, &commontypes.DIError{
			Type:    commontypes.ErrorBadRequest,
			Message: fmt.Sprintf("request generation %s is not a valid number", items[2]),
		}
	}
	return items[0], items[1], gen, nil
}
