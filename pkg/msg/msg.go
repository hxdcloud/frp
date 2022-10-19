// Copyright 2016 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msg

import (
	"net"
)

const (
	TypeLogin                 = 'o'
	TypeLoginResp             = '1'
	TypeNewProxy              = 'p'
	TypeNewProxyResp          = '2'
	TypeNewProxyIni           = '6'
	TypeCloseProxy            = 'c'
	TypeNewWorkConn           = 'w'
	TypeReqWorkConn           = 'r'
	TypeStartWorkConn         = 's'
	TypeNewVisitorConn        = 'v'
	TypeNewVisitorConnResp    = '3'
	TypePing                  = 'h'
	TypePong                  = '4'
	TypeUDPPacket             = 'u'
	TypeNatHoleVisitor        = 'i'
	TypeNatHoleClient         = 'n'
	TypeNatHoleResp           = 'm'
	TypeNatHoleClientDetectOK = 'd'
	TypeNatHoleSid            = '5'
)

var msgTypeMap = map[byte]interface{}{
	TypeLogin:                 Login{},
	TypeLoginResp:             LoginResp{},
	TypeNewProxy:              NewProxy{},
	TypeNewProxyResp:          NewProxyResp{},
	TypeNewProxyIni:           NewProxyIni{},
	TypeCloseProxy:            CloseProxy{},
	TypeNewWorkConn:           NewWorkConn{},
	TypeReqWorkConn:           ReqWorkConn{},
	TypeStartWorkConn:         StartWorkConn{},
	TypeNewVisitorConn:        NewVisitorConn{},
	TypeNewVisitorConnResp:    NewVisitorConnResp{},
	TypePing:                  Ping{},
	TypePong:                  Pong{},
	TypeUDPPacket:             UDPPacket{},
	TypeNatHoleVisitor:        NatHoleVisitor{},
	TypeNatHoleClient:         NatHoleClient{},
	TypeNatHoleResp:           NatHoleResp{},
	TypeNatHoleClientDetectOK: NatHoleClientDetectOK{},
	TypeNatHoleSid:            NatHoleSid{},
}

// When frpc start, client send this message to login to server.
type Login struct {
	Version      string            `json:"version,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	Os           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	User         string            `json:"user,omitempty"`
	PrivilegeKey string            `json:"privilege_key,omitempty"`
	Timestamp    int64             `json:"timestamp,omitempty"`
	RunID        string            `json:"run_id,omitempty"`
	Metas        map[string]string `json:"metas,omitempty"`

	// Some global configures.
	PoolCount int `json:"pool_count,omitempty"`
}

type LoginResp struct {
	Version       string `json:"version,omitempty"`
	RunID         string `json:"run_id,omitempty"`
	ServerUDPPort int    `json:"server_udp_port,omitempty"`
	Error         string `json:"error,omitempty"`
}

// When frpc login success, send this message to frps for running a new proxy.
type NewProxy struct {
	ProxyName      string            `json:"proxy_name,omitempty"`
	ProxyType      string            `json:"proxy_type,omitempty"`
	UseEncryption  bool              `json:"use_encryption,omitempty"`
	UseCompression bool              `json:"use_compression,omitempty"`
	Group          string            `json:"group,omitempty"`
	GroupKey       string            `json:"group_key,omitempty"`
	Metas          map[string]string `json:"metas,omitempty"`

	// tcp and udp only
	RemotePort int `json:"remote_port,omitempty"`

	// http and https only
	CustomDomains     []string          `json:"custom_domains,omitempty"`
	SubDomain         string            `json:"subdomain,omitempty"`
	Locations         []string          `json:"locations,omitempty"`
	HTTPUser          string            `json:"http_user,omitempty"`
	HTTPPwd           string            `json:"http_pwd,omitempty"`
	HostHeaderRewrite string            `json:"host_header_rewrite,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	RouteByHTTPUser   string            `json:"route_by_http_user,omitempty"`

	// stcp
	Sk string `json:"sk,omitempty"`

	// tcpmux
	Multiplexer string `json:"multiplexer,omitempty"`

	// LocalIP specifies the IP address or host name to to.
	LocalIP string `ini:"local_ip" json:"local_ip"`
	// LocalPort specifies the port to to.
	LocalPort int `ini:"local_port" json:"local_port"`
}

type NewProxyResp struct {
	ProxyName  string `json:"proxy_name,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	Error      string `json:"error,omitempty"`
}

type CloseProxy struct {
	ProxyName string `json:"proxy_name,omitempty"`
}

type NewWorkConn struct {
	RunID        string `json:"run_id,omitempty"`
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type ReqWorkConn struct{}

type StartWorkConn struct {
	ProxyName string `json:"proxy_name,omitempty"`
	SrcAddr   string `json:"src_addr,omitempty"`
	DstAddr   string `json:"dst_addr,omitempty"`
	SrcPort   uint16 `json:"src_port,omitempty"`
	DstPort   uint16 `json:"dst_port,omitempty"`
	Error     string `json:"error,omitempty"`
}

type NewVisitorConn struct {
	ProxyName      string `json:"proxy_name,omitempty"`
	SignKey        string `json:"sign_key,omitempty"`
	Timestamp      int64  `json:"timestamp,omitempty"`
	UseEncryption  bool   `json:"use_encryption,omitempty"`
	UseCompression bool   `json:"use_compression,omitempty"`
}

type NewVisitorConnResp struct {
	ProxyName string `json:"proxy_name,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Ping struct {
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type Pong struct {
	Error string `json:"error,omitempty"`
}

type UDPPacket struct {
	Content    string       `json:"c,omitempty"`
	LocalAddr  *net.UDPAddr `json:"l,omitempty"`
	RemoteAddr *net.UDPAddr `json:"r,omitempty"`
}

type NatHoleVisitor struct {
	ProxyName string `json:"proxy_name,omitempty"`
	SignKey   string `json:"sign_key,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

type NatHoleClient struct {
	ProxyName string `json:"proxy_name,omitempty"`
	Sid       string `json:"sid,omitempty"`
}

type NatHoleResp struct {
	Sid         string `json:"sid,omitempty"`
	VisitorAddr string `json:"visitor_addr,omitempty"`
	ClientAddr  string `json:"client_addr,omitempty"`
	Error       string `json:"error,omitempty"`
}

type NatHoleClientDetectOK struct{}

type NatHoleSid struct {
	Sid string `json:"sid,omitempty"`
}

type NewProxyIni struct {

	// RunId client unique id
	RunId string `ini:"run_id" json:"run_id"`

	// ProxyName is the name of this
	ProxyName string `ini:"name" json:"name"`
	// ProxyType specifies the type of this  Valid values include "tcp",
	// "udp", "http", "https", "stcp", and "xtcp". By default, this value is
	// "tcp".
	ProxyType string `ini:"type" json:"type"`

	// UseEncryption controls whether or not communication with the server will
	// be encrypted. Encryption is done using the tokens supplied in the server
	// and client configuration. By default, this value is false.
	UseEncryption string `ini:"use_encryption" json:"use_encryption"`
	// UseCompression controls whether or not communication with the server
	// will be compressed. By default, this value is false.
	UseCompression string `ini:"use_compression" json:"use_compression"`
	// Group specifies which group the is a part of. The server will use
	// this information to load balance proxies in the same group. If the value
	// is "", this will not be in a group. By default, this value is "".
	Group string `ini:"group" json:"group"`
	// GroupKey specifies a group key, which should be the same among proxies
	// of the same group. By default, this value is "".
	GroupKey string `ini:"group_key" json:"group_key"`

	// ProxyProtocolVersion specifies which protocol version to use. Valid
	// values include "v1", "v2", and "". If the value is "", a protocol
	// version will be automatically selected. By default, this value is "".
	ProxyProtocolVersion string `ini:"proxy_protocol_version" json:"proxy_protocol_version"`

	// BandwidthLimit limit the bandwidth
	// 0 means no limit
	BandwidthLimit string `ini:"bandwidth_limit" json:"bandwidth_limit"`

	// meta info for each proxy
	Metas string `ini:"-" json:"metas"`

	// LocalIP specifies the IP address or host name to to.
	LocalIP string `ini:"local_ip" json:"local_ip"`
	// LocalPort specifies the port to to.
	LocalPort string `ini:"local_port" json:"local_port"`

	// Plugin specifies what plugin should be used for ng. If this value
	// is set, the LocalIp and LocalPort values will be ignored. By default,
	// this value is "".
	Plugin string `ini:"plugin" json:"plugin"`
	// PluginParams specify parameters to be passed to the plugin, if one is
	// being used. By default, this value is an empty map.
	PluginParams string `ini:"-"`

	// HealthCheckType specifies what protocol to use for health checking.
	// Valid values include "tcp", "http", and "". If this value is "", health
	// checking will not be performed. By default, this value is "".
	//
	// If the type is "tcp", a connection will be attempted to the target
	// server. If a connection cannot be established, the health check fails.
	//
	// If the type is "http", a GET request will be made to the endpoint
	// specified by HealthCheckURL. If the response is not a 200, the health
	// check fails.
	HealthCheckType string `ini:"health_check_type" json:"health_check_type"` // tcp | http
	// HealthCheckTimeoutS specifies the number of seconds to wait for a health
	// check attempt to connect. If the timeout is reached, this counts as a
	// health check failure. By default, this value is 3.
	HealthCheckTimeoutS string `ini:"health_check_timeout_s" json:"health_check_timeout_s"`
	// HealthCheckMaxFailed specifies the number of allowed failures before the
	// is stopped. By default, this value is 1.
	HealthCheckMaxFailed string `ini:"health_check_max_failed" json:"health_check_max_failed"`
	// HealthCheckIntervalS specifies the time in seconds between health
	// checks. By default, this value is 10.
	HealthCheckIntervalS string `ini:"health_check_interval_s" json:"health_check_interval_s"`
	// HealthCheckURL specifies the address to send health checks to if the
	// health check type is "http".
	HealthCheckURL string `ini:"health_check_url" json:"health_check_url"`
	// HealthCheckAddr specifies the address to connect to if the health check
	// type is "tcp".
	HealthCheckAddr string `ini:"-"`

	CustomDomains string `ini:"custom_domains" json:"custom_domains"`
	SubDomain     string `ini:"subdomain" json:"subdomain"`

	Locations         string `ini:"locations" json:"locations"`
	HTTPUser          string `ini:"http_user" json:"http_user"`
	HTTPPwd           string `ini:"http_pwd" json:"http_pwd"`
	HostHeaderRewrite string `ini:"host_header_rewrite" json:"host_header_rewrite"`
	Headers           string `ini:"-" json:"headers"`
	RouteByHTTPUser   string `ini:"route_by_http_user" json:"route_by_http_user"`

	RemotePort string `ini:"remote_port" json:"remote_port"`

	Multiplexer string `ini:"multiplexer"`

	Role string `ini:"role" json:"role"`
	Sk   string `ini:"sk" json:"sk"`
}

type BandwidthQuantity struct {
	s string // MB or KB

	i int64 // bytes
}
