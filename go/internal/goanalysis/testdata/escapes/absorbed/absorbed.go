package absorbed

type Handlers struct {
	Open func()
}

func Load() {}
func Save() {}

type Backend struct{}

func (b *Backend) Read() {}
