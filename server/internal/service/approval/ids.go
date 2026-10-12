package approval

import (
	"strings"

	"github.com/google/uuid"
)

func newID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}
