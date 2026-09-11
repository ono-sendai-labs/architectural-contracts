package consumer

import "example.com/injected/runtime"

// MarkerValue proves that the checked member requires the hidden runtime's
// export data for both import resolution and type loading.
var MarkerValue runtime.Marker
