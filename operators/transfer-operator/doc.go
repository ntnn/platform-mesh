// transport-operator manages copying resource .spec and .status between control planes.
//
// The *Provider resources tell the operator how to feed target control
// planes into the operator. Note that currently only the KubeconfigProvider is implemented.
// Note that at present the *Provider is expected to yield one cluster to have a 1:1 relationship.
//
// The Transfer resources tell the operator which resources to transfer
// to which control planes.
//
// In a default setup with kcp the flow is as follows:
//
// 1. APIExport export *Provider and Transport
// 2. Consumer binds
// 3. Consumer creates *Provider
// 4. transport-operator sees *Provider and engages its clusters
// 5. Consumer creates Transport targeting a *Provider
// 6. transport-operator sees Transport, verifies *Provider exists
// 6.1. ... sets up watches for the resources noted in Transport in consumer Workspace
// 6.2. ... sets up watches for the resources noted in Transport in clusters engaged by *Provider
// 6.3. ... transport the targeted resources' spec and status between consumer Workspace and engaged clusters by *Provider
package main
