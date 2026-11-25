package integration

import "vaultly/internal/apperr"

func invalid(err error) error  { return apperr.Invalid(err.Error()) }
func upstream(err error) error { return apperr.Upstream(err.Error()) }
