// Package atoha defines execution semantics for actions that cross into
// physical systems.
//
// Its API keeps immutable intent, individual execution attempts, physical
// outcome knowledge, and software-path errors distinct. Integrations and
// domain policy are supplied at the edges rather than embedded in core.
package atoha
