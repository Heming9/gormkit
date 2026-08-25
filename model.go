package gormkit

import "encoding/json"

// Paging describes offset-based pagination and receives the total row count.
type Paging interface {
	Offset() int
	Limit() int
	SetTotal(total int64)
}

// Query applies a reusable query fragment.
type Query interface {
	Apply(db Client) Client
}

// JSONData provides explicit JSON encoding helpers for byte slices.
type JSONData []byte

// Marshal replaces the receiver with the JSON representation of data.
func (data *JSONData) Marshal(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	*data = append((*data)[:0], encoded...)
	return nil
}

// Unmarshal decodes the receiver into target. Empty data is a no-op.
func (data JSONData) Unmarshal(target any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

// JsonData is retained for source compatibility.
// Deprecated: use JSONData.
type JsonData = JSONData

// Timestamp is a Unix timestamp in seconds.
type Timestamp = int64
