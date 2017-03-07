package rtp

// All rtp protocol errors are defined here.

type rtpError struct {
	message string
}

func (e *rtpError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.message
}

func (e *rtpError) responseError() string {
	return "Response: " + e.Error()
}

var (
	zeroLengthMsg       = &rtpError{message: "Message's length was set to zero"}
	unexpectedLengthMsg = &rtpError{message: "Unexpected message data length"}
	incompleteMsg       = &rtpError{message: "Message's data is incomplete"}
	unknownVersionMsg   = &rtpError{message: "Unexpected RTP Version"}
)
