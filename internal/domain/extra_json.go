package domain

// JSON methods that keep unknown fields (see Extra). Each one converts to a
// local "plain" type with the same fields but no methods, to avoid recursion.

// MarshalJSON encodes the series and its unknown fields.
func (s Series) MarshalJSON() ([]byte, error) {
	type plain Series
	return MarshalWithExtra(plain(s), s.Extra)
}

// UnmarshalJSON decodes the series and keeps unknown fields in Extra.
func (s *Series) UnmarshalJSON(b []byte) error {
	type plain Series
	var p plain
	extra, err := UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*s = Series(p)
	s.Extra = extra
	return nil
}

// MarshalJSON encodes the chapter and its unknown fields.
func (c Chapter) MarshalJSON() ([]byte, error) {
	type plain Chapter
	return MarshalWithExtra(plain(c), c.Extra)
}

// UnmarshalJSON decodes the chapter and keeps unknown fields in Extra.
func (c *Chapter) UnmarshalJSON(b []byte) error {
	type plain Chapter
	var p plain
	extra, err := UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*c = Chapter(p)
	c.Extra = extra
	return nil
}

// MarshalJSON encodes the source and its unknown fields.
func (s Source) MarshalJSON() ([]byte, error) {
	type plain Source
	return MarshalWithExtra(plain(s), s.Extra)
}

// UnmarshalJSON decodes the source and keeps unknown fields in Extra.
func (s *Source) UnmarshalJSON(b []byte) error {
	type plain Source
	var p plain
	extra, err := UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*s = Source(p)
	s.Extra = extra
	return nil
}

// MarshalJSON encodes the cover and its unknown fields.
func (c Cover) MarshalJSON() ([]byte, error) {
	type plain Cover
	return MarshalWithExtra(plain(c), c.Extra)
}

// UnmarshalJSON decodes the cover and keeps unknown fields in Extra.
func (c *Cover) UnmarshalJSON(b []byte) error {
	type plain Cover
	var p plain
	extra, err := UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*c = Cover(p)
	c.Extra = extra
	return nil
}
