package translator

type Warning struct {
	Message string
}

func (w *Warning) Error() string {
	return w.Message
}
func (w *Warning) Is(err error) bool {
	_, ok := err.(*Warning)
	return ok
}
func (w *Warning) As(err any) bool {
	_, ok := err.(*Warning)
	return ok
}

var _ error = new(Warning)
