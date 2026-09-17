package adlast

type ATypeExpr[T any] struct {
	_ATypeExpr[T]
}

type _ATypeExpr[T any] struct {
	Value TypeExpr `json:"value"`
}

func MakeAll_ATypeExpr[T any](
	value TypeExpr,
) ATypeExpr[T] {
	return ATypeExpr[T]{
		_ATypeExpr[T]{
			Value: value,
		},
	}
}

func Make_ATypeExpr[T any](
	value TypeExpr,
) ATypeExpr[T] {
	ret := ATypeExpr[T]{
		_ATypeExpr[T]{
			Value: value,
		},
	}
	return ret
}
