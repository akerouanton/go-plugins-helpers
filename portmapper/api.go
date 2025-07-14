package portmapper

import (
	"net/http"
	"net/netip"

	"github.com/docker/go-plugins-helpers/sdk"
)

const (
	manifest = `{"Implements": ["PortMapper"]}`

	mapPortsPath   = "/PortMapper.MapPorts"
	unmapPortsPath = "/PortMapper.UnmapPorts"
)

type Driver interface {
	MapPorts(MapPortsRequest) (MapPortsResponse, error)
	UnmapPorts(UnmapPortsRequest) error
}

// MapPortsRequest is the API request sent to the PortMapper plugin to map a port.
type MapPortsRequest struct {
	// Reqs is a group of port bindings that should get the same port assigned
	// when an ephemeral port, or a port range is requested. Each Reqs should
	// yield an independent PortBinding.
	Reqs []PortBindingReq

	// Labels is a set of opaque, user-specified freeform labels that can be
	// used to tweak how the port-mapper behaves.
	Labels map[string]string
}

type PortBindingReq struct {
	Proto Protocol

	BackendIP   netip.Addr
	BackendPort uint16

	FrontendIP netip.Addr
	// FrontendPort is either an exact port, an ephemeral port (= 0), or the start
	// of a port range.
	FrontendPort uint16
	// FrontendPortEnd should be the same as HostPort when an exact or ephemeral
	// port is requested. Otherwise, it should be the end of the port range.
	FrontendPortEnd uint16

	// ExtraParams is a map of extra parameters passed to the mapper to map and
	// unmap this port binding.
	ExtraParams map[string]string
}

type MapPortsResponse struct {
	PortBindings []PortBinding
	Err          string
}

// PortBinding is returned by the PortMapper when a request has been processed.
type PortBinding struct {
	Proto Protocol

	BackendIP   netip.Addr
	BackendPort uint16

	FrontendIP netip.Addr
	// FrontendPort is the frontend port picked by the PortMapper when an
	// ephemeral port, or a port range was specified in the request.
	FrontendPort uint16

	// ExtraParams is a map of extra parameters passed to the mapper to map and
	// unmap this port binding.
	ExtraParams map[string]string

	// DiscardReq is returned when the PortBindingReq can't be fulfilled, but
	// the driver deems it's not a fatal error.
	DiscardReq bool

	// NAT is returned by the portmapper when it needs the Engine to configure
	// the host firewall to NAT a port to BackendIP:BackendPort.
	NAT netip.AddrPort
	// Forwarding indicates whether the host firewall should be reconfigured to
	// allow forwarding to BackendIP:BackendPort.
	Forwarding bool

	// Error indicates the reason why this port binding was discarded. This
	// error is logged.
	Error string
}

type Protocol int

const (
	ProtocolTCP  Protocol = 0x6
	ProtocolUDP  Protocol = 0x11
	ProtocolSCTP Protocol = 0x84
)

func (p Protocol) String() string {
	switch p {
	case ProtocolTCP:
		return "tcp"
	case ProtocolUDP:
		return "udp"
	case ProtocolSCTP:
		return "sctp"
	default:
		return "unknown"
	}
}

type UnmapPortsRequest struct {
	PortBindings []PortBinding
	Labels       map[string]string
}

// ErrorResponse is a formatted error message that libnetwork can understand
type ErrorResponse struct {
	Err string
}

// NewErrorResponse creates an ErrorResponse with the provided message
func NewErrorResponse(msg string) *ErrorResponse {
	return &ErrorResponse{Err: msg}
}

// Handler forwards requests and responses between the docker daemon and the plugin.
type Handler struct {
	driver Driver
	*sdk.Handler
}

// NewHandler initializes the request handler with a driver implementation.
func NewHandler(driver Driver) *Handler {
	h := &Handler{driver, sdk.NewHandler(manifest)}
	h.initMux()
	return h
}

func (h *Handler) initMux() {
	h.HandleFunc(mapPortsPath, func(w http.ResponseWriter, r *http.Request) {
		req := &MapPortsRequest{}
		err := sdk.DecodeRequest(w, r, req)
		if err != nil {
			return
		}
		res, err := h.driver.MapPorts(*req)
		if err != nil {
			sdk.EncodeResponse(w, NewErrorResponse(err.Error()), true)
			return
		}
		sdk.EncodeResponse(w, res, false)
	})
	h.HandleFunc(unmapPortsPath, func(w http.ResponseWriter, r *http.Request) {
		req := &UnmapPortsRequest{}
		err := sdk.DecodeRequest(w, r, req)
		if err != nil {
			return
		}
		err = h.driver.UnmapPorts(*req)
		if err != nil {
			sdk.EncodeResponse(w, NewErrorResponse(err.Error()), true)
			return
		}
		sdk.EncodeResponse(w, struct{}{}, false)
	})
}
