//go:build wasip1

// Backend of the UPS monitoring plugin. Polls a NUT server every 60 s and
// writes charge and runtime into the ups entity. Uses the phoedox-sdk Go PDK
// once the host API bindings are available.
package main

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	// TODO: pdk.Secret("nut_password"), pdk.Fetch(...), pdk.Graph.Upsert("ups", ...)
	return 0
}
