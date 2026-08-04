// Package problems owns the public problem catalog and the server-only judge
// definition used to execute it.
//
// A case's Hidden field is the authority for exposure. Hidden inputs and
// expected values must never be returned by a problem endpoint before a
// submission. After a full submission, a failing hidden case may disclose its
// expected and received values in the submission result; that feedback is
// intentional for this practice product and must be truncated before it is
// returned so stress cases cannot produce unbounded responses.
package problems
