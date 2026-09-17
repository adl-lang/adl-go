package goadl

type Void struct{}

func (obj *Void) UnmarshalJSON(b []byte) error {
	return nil
}

func (obj Void) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}
