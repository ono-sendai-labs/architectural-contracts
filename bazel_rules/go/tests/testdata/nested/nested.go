package nested

import "example.com/aspect/nested/child"

func Name() string { return "nested/" + child.Name() }
