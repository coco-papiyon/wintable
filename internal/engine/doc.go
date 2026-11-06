// Package engine will coordinate validated inventory, modules and injected
// transport.Connector implementations, then aggregate result.TaskResult values.
// Host order, concurrency, cancellation and resource cleanup belong here.
// Neither transport nor result imports engine.
package engine
