package crid

import "strconv"

// ID is a 63-bit Snowflake-style distributed identifier.
type ID int64

// Int64 returns the ID as a plain int64.
func (id ID) Int64() int64 {
	return int64(id)
}

// String returns the ID as a decimal string.
func (id ID) String() string {
	return strconv.FormatInt(int64(id), 10)
}

// MarshalJSON encodes the ID as a quoted decimal string to avoid precision loss in JavaScript,
// which cannot represent 63-bit integers exactly.
func (id ID) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 22)
	b = append(b, '"')
	b = strconv.AppendInt(b, int64(id), 10)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON decodes the ID from a quoted decimal string.
func (id *ID) UnmarshalJSON(data []byte) error {
	s, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*id = ID(i)
	return nil
}
