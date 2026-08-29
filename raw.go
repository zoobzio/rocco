package rocco

// ContentTypeOctetStream is the default media type for raw bodies.
const ContentTypeOctetStream = "application/octet-stream"

// Blob represents a raw binary response.
// Return this from a handler to write bytes as-is with a per-response
// Content-Type, bypassing the handler codec. Use WithMediaTypes to declare
// the possible content types in the OpenAPI spec.
//
// Example:
//
//	handler := rocco.GET[rocco.NoBody, rocco.Blob]("/assets/{id}", func(req *rocco.Request[rocco.NoBody]) (rocco.Blob, error) {
//	    asset := load(req.Params.Path["id"])
//	    return rocco.Blob{ContentType: asset.MIME, Data: asset.Bytes}, nil
//	}).WithMediaTypes("image/png", "application/pdf")
type Blob struct {
	ContentType string // Response Content-Type (default: first declared media type)
	Data        []byte // Raw response body
	Status      int    // HTTP status code (default: handler success status)
}

// RawBody represents a raw binary request body.
// Use it as the input type to receive the body bytes without codec decoding.
// rocco fills ContentType from the incoming Content-Type header and Data with
// the body bytes. WithMaxBodySize still applies.
//
// Example:
//
//	handler := rocco.POST[rocco.RawBody, UploadResult]("/upload", func(req *rocco.Request[rocco.RawBody]) (UploadResult, error) {
//	    return store(req.Body.ContentType, req.Body.Data)
//	})
type RawBody struct {
	ContentType string // Incoming Content-Type header
	Data        []byte // Raw request body
}
