package directd

import (
	"github.com/arandu-io/framework/foundation"
	fhttp "github.com/arandu-io/framework/http"
)

// Compile-time proof that the client honors the contract it claims.
var _ foundation.Module = (*Client)(nil)

// Name is the module identifier: a lowercase slug, stable, no spaces.
func (c *Client) Name() string { return "directd" }

// Routes registers nothing.
//
// A query costs money and returns personal data, so it is made through the
// application's own controllers, behind its Grants and Policies, with a Meter
// bound to the tenant the Grant names. A route here would query any document
// for anybody who sent one, on the application's balance. The asynchronous
// callback is the application's route too: it calls ParseCallback and
// CompleteAsync.
func (c *Client) Routes(*fhttp.Router) {}
