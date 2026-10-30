package antigravity

// NewSearchLineFilter implements thinkt.SearchLineFilterer with
// NewSearchFilter, which must see every line of the file in order.
func (d *Discoverer) NewSearchLineFilter() func(line []byte, matched bool) bool {
	keep := NewSearchFilter()
	return func(line []byte, _ bool) bool { return keep(line) }
}
