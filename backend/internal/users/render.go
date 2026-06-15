package users

// IsEffectivelyActive reports whether a user may currently connect: it is
// enabled AND not past its expiry. ExpiresAt==0 means "never expires". Time is
// unix seconds; at the exact boundary now==ExpiresAt the user is EXPIRED
// (the comparison is strict now < ExpiresAt). PURE.
func IsEffectivelyActive(u PanelUser, now int64) bool {
	return u.Enabled && (u.ExpiresAt == 0 || now < u.ExpiresAt)
}
