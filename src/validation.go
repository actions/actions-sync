package src

import (
	"strings"

	"github.com/pkg/errors"
)

type Validations []string

func (v Validations) Error() error {
	if len(v) <= 0 {
		return nil
	}
	return errors.New("Error (Invalid Flags):\n  " + strings.Join(v, "\n  ") + "\n")
}

func (v Validations) Join(o Validations) Validations {
	return append(v, o...)
}
