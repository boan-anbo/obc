package ob

import "openbibliographies/ob/internal/searchnorm"

// LegacySemanticsSHA256 is the reference closure of scripts/export-reference.mjs; see
// testdata/reference-binding.json. The closure predates the shared normalisation.
const LegacySemanticsSHA256 = "fb3ecac2995906cd2dc3cdf02a467c358d33380abf7cc23926e7959c1d5587ce"

// SemanticsSHA256 also covers the shared normalisation table: a changed table changes
// what a query finds, so it is a new reader, not a data update.
var SemanticsSHA256 = Digest([]byte(LegacySemanticsSHA256 + "\n" + searchnorm.TableSHA256 + "\n"))
