package inner

// generate:reset
type Item struct {
	Name  string
	Count *int
}

func (it Item) String() string {
	return it.Name
}
