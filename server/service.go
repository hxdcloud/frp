// Copyright 2017 fatedier, fatedier@gmail.com
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

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/fatedier/golib/net/mux"
	fmux "github.com/hashicorp/yamux"

	"github.com/fatedier/frp/assets"
	"github.com/fatedier/frp/pkg/auth"
	"github.com/fatedier/frp/pkg/config"
	modelmetrics "github.com/fatedier/frp/pkg/metrics"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/nathole"
	plugin "github.com/fatedier/frp/pkg/plugin/server"
	"github.com/fatedier/frp/pkg/transport"
	"github.com/fatedier/frp/pkg/util/log"
	frpNet "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/pkg/util/tcpmux"
	"github.com/fatedier/frp/pkg/util/util"
	"github.com/fatedier/frp/pkg/util/version"
	"github.com/fatedier/frp/pkg/util/vhost"
	"github.com/fatedier/frp/pkg/util/xlog"
	"github.com/fatedier/frp/server/controller"
	"github.com/fatedier/frp/server/group"
	"github.com/fatedier/frp/server/metrics"
	"github.com/fatedier/frp/server/ports"
	"github.com/fatedier/frp/server/proxy"
	"github.com/fatedier/frp/server/visitor"
)

const (
	connReadTimeout       time.Duration = 10 * time.Second
	vhostReadWriteTimeout time.Duration = 30 * time.Second
)

// Server service
type Service struct {
	// Dispatch connections to different handlers listen on same port
	muxer *mux.Mux

	// Accept connections from client
	listener net.Listener

	// Accept connections using kcp
	kcpListener net.Listener

	// Accept connections using websocket
	websocketListener net.Listener

	// Accept frp tls connections
	tlsListener net.Listener

	// Manage all controllers
	CtlManager *ControlManager

	// Manage all proxies
	PxyManager *proxy.Manager

	// Manage all plugins
	pluginManager *plugin.Manager

	// HTTP vhost router
	httpVhostRouter *vhost.Routers

	// All resource managers and controllers
	rc *controller.ResourceController

	// Verifies authentication based on selected method
	authVerifier auth.Verifier

	tlsConfig *tls.Config

	Cfg config.ServerCommonConf
}

func NewService(Cfg config.ServerCommonConf) (svr *Service, err error) {
	tlsConfig, err := transport.NewServerTLSConfig(
		Cfg.TLSCertFile,
		Cfg.TLSKeyFile,
		Cfg.TLSTrustedCaFile)
	if err != nil {
		return
	}

	svr = &Service{
		CtlManager:    NewControlManager(),
		PxyManager:    proxy.NewManager(),
		pluginManager: plugin.NewManager(),
		rc: &controller.ResourceController{
			VisitorManager: visitor.NewManager(),
			TCPPortManager: ports.NewManager("tcp", Cfg.ProxyBindAddr, Cfg.AllowPorts),
			UDPPortManager: ports.NewManager("udp", Cfg.ProxyBindAddr, Cfg.AllowPorts),
		},
		httpVhostRouter: vhost.NewRouters(),
		authVerifier:    auth.NewAuthVerifier(Cfg.ServerConfig),
		tlsConfig:       tlsConfig,
		Cfg:             Cfg,
	}

	// Create tcpmux httpconnect multiplexer.
	if Cfg.TCPMuxHTTPConnectPort > 0 {
		var l net.Listener
		address := net.JoinHostPort(Cfg.ProxyBindAddr, strconv.Itoa(Cfg.TCPMuxHTTPConnectPort))
		l, err = net.Listen("tcp", address)
		if err != nil {
			err = fmt.Errorf("create server listener error, %v", err)
			return
		}

		svr.rc.TCPMuxHTTPConnectMuxer, err = tcpmux.NewHTTPConnectTCPMuxer(l, Cfg.TCPMuxPassthrough, vhostReadWriteTimeout)
		if err != nil {
			err = fmt.Errorf("create vhost tcpMuxer error, %v", err)
			return
		}
		log.Info("tcpmux httpconnect multiplexer listen on %s, passthough: %v", address, Cfg.TCPMuxPassthrough)
	}

	// Init all plugins
	pluginNames := make([]string, 0, len(Cfg.HTTPPlugins))
	for n := range Cfg.HTTPPlugins {
		pluginNames = append(pluginNames, n)
	}
	sort.Strings(pluginNames)

	for _, name := range pluginNames {
		svr.pluginManager.Register(plugin.NewHTTPPluginOptions(Cfg.HTTPPlugins[name]))
		log.Info("plugin [%s] has been registered", name)
	}
	svr.rc.PluginManager = svr.pluginManager

	// Init group controller
	svr.rc.TCPGroupCtl = group.NewTCPGroupCtl(svr.rc.TCPPortManager)

	// Init HTTP group controller
	svr.rc.HTTPGroupCtl = group.NewHTTPGroupController(svr.httpVhostRouter)

	// Init TCP mux group controller
	svr.rc.TCPMuxGroupCtl = group.NewTCPMuxGroupCtl(svr.rc.TCPMuxHTTPConnectMuxer)

	// Init 404 not found page
	vhost.NotFoundPagePath = Cfg.Custom404Page

	var (
		httpMuxOn  bool
		httpsMuxOn bool
	)
	if Cfg.BindAddr == Cfg.ProxyBindAddr {
		if Cfg.BindPort == Cfg.VhostHTTPPort {
			httpMuxOn = true
		}
		if Cfg.BindPort == Cfg.VhostHTTPSPort {
			httpsMuxOn = true
		}
	}

	// Listen for accepting connections from client.
	address := net.JoinHostPort(Cfg.BindAddr, strconv.Itoa(Cfg.BindPort))
	ln, err := net.Listen("tcp", address)
	if err != nil {
		err = fmt.Errorf("create server listener error, %v", err)
		return
	}

	svr.muxer = mux.NewMux(ln)
	svr.muxer.SetKeepAlive(time.Duration(Cfg.TCPKeepAlive) * time.Second)
	go func() {
		_ = svr.muxer.Serve()
	}()
	ln = svr.muxer.DefaultListener()

	svr.listener = ln
	log.Info("frps tcp listen on %s", address)

	// Listen for accepting connections from client using kcp protocol.
	if Cfg.KCPBindPort > 0 {
		address := net.JoinHostPort(Cfg.BindAddr, strconv.Itoa(Cfg.KCPBindPort))
		svr.kcpListener, err = frpNet.ListenKcp(address)
		if err != nil {
			err = fmt.Errorf("listen on kcp address udp %s error: %v", address, err)
			return
		}
		log.Info("frps kcp listen on udp %s", address)
	}

	// Listen for accepting connections from client using websocket protocol.
	websocketPrefix := []byte("GET " + frpNet.FrpWebsocketPath)
	websocketLn := svr.muxer.Listen(0, uint32(len(websocketPrefix)), func(data []byte) bool {
		return bytes.Equal(data, websocketPrefix)
	})
	svr.websocketListener = frpNet.NewWebsocketListener(websocketLn)

	// Create http vhost muxer.
	if Cfg.VhostHTTPPort > 0 {
		rp := vhost.NewHTTPReverseProxy(vhost.HTTPReverseProxyOptions{
			ResponseHeaderTimeoutS: Cfg.VhostHTTPTimeout,
		}, svr.httpVhostRouter)
		svr.rc.HTTPReverseProxy = rp

		address := net.JoinHostPort(Cfg.ProxyBindAddr, strconv.Itoa(Cfg.VhostHTTPPort))
		server := &http.Server{
			Addr:    address,
			Handler: rp,
		}
		var l net.Listener
		if httpMuxOn {
			l = svr.muxer.ListenHttp(1)
		} else {
			l, err = net.Listen("tcp", address)
			if err != nil {
				err = fmt.Errorf("create vhost http listener error, %v", err)
				return
			}
		}
		go func() {
			_ = server.Serve(l)
		}()
		log.Info("http service listen on %s", address)
	}

	// Create https vhost muxer.
	if Cfg.VhostHTTPSPort > 0 {
		var l net.Listener
		if httpsMuxOn {
			l = svr.muxer.ListenHttps(1)
		} else {
			address := net.JoinHostPort(Cfg.ProxyBindAddr, strconv.Itoa(Cfg.VhostHTTPSPort))
			l, err = net.Listen("tcp", address)
			if err != nil {
				err = fmt.Errorf("create server listener error, %v", err)
				return
			}
			log.Info("https service listen on %s", address)
		}

		svr.rc.VhostHTTPSMuxer, err = vhost.NewHTTPSMuxer(l, vhostReadWriteTimeout)
		if err != nil {
			err = fmt.Errorf("create vhost httpsMuxer error, %v", err)
			return
		}
	}

	// frp tls listener
	svr.tlsListener = svr.muxer.Listen(2, 1, func(data []byte) bool {
		// tls first byte can be 0x16 only when vhost https port is not same with bind port
		return int(data[0]) == frpNet.FRPTLSHeadByte || int(data[0]) == 0x16
	})

	// Create nat hole controller.
	if Cfg.BindUDPPort > 0 {
		var nc *nathole.Controller
		address := net.JoinHostPort(Cfg.BindAddr, strconv.Itoa(Cfg.BindUDPPort))
		nc, err = nathole.NewController(address)
		if err != nil {
			err = fmt.Errorf("create nat hole controller error, %v", err)
			return
		}
		svr.rc.NatHoleController = nc
		log.Info("nat hole udp service listen on %s", address)
	}

	var statsEnable bool
	// Create dashboard web server.
	if Cfg.DashboardPort > 0 {
		// Init dashboard assets
		assets.Load(Cfg.AssetsDir)

		address := net.JoinHostPort(Cfg.DashboardAddr, strconv.Itoa(Cfg.DashboardPort))
		err = svr.RunDashboardServer(address)
		if err != nil {
			err = fmt.Errorf("create dashboard web server error, %v", err)
			return
		}
		log.Info("Dashboard listen on %s", address)
		statsEnable = true
	}
	if statsEnable {
		modelmetrics.EnableMem()
		if Cfg.EnablePrometheus {
			modelmetrics.EnablePrometheus()
		}
	}
	return
}

func (svr *Service) Run() {
	if svr.rc.NatHoleController != nil {
		go svr.rc.NatHoleController.Run()
	}
	if svr.Cfg.KCPBindPort > 0 {
		go svr.HandleListener(svr.kcpListener)
	}

	go svr.HandleListener(svr.websocketListener)
	go svr.HandleListener(svr.tlsListener)

	svr.HandleListener(svr.listener)
}

func (svr *Service) handleConnection(ctx context.Context, conn net.Conn) {
	xl := xlog.FromContextSafe(ctx)

	var (
		rawMsg msg.Message
		err    error
	)

	_ = conn.SetReadDeadline(time.Now().Add(connReadTimeout))
	if rawMsg, err = msg.ReadMsg(conn); err != nil {
		log.Trace("Failed to read message: %v", err)
		conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	switch m := rawMsg.(type) {
	case *msg.Login:
		// server plugin hook
		content := &plugin.LoginContent{
			Login:         *m,
			ClientAddress: conn.RemoteAddr().String(),
		}
		retContent, err := svr.pluginManager.Login(content)
		if err == nil {
			m = &retContent.Login
			err = svr.RegisterControl(conn, m)
		}

		// If login failed, send error message there.
		// Otherwise send success message in control's work goroutine.
		if err != nil {
			xl.Warn("register control error: %v", err)
			_ = msg.WriteMsg(conn, &msg.LoginResp{
				Version: version.Full(),
				Error:   util.GenerateResponseErrorString("register control error", err, svr.Cfg.DetailedErrorsToClient),
			})
			conn.Close()
		}
	case *msg.NewWorkConn:
		if err := svr.RegisterWorkConn(conn, m); err != nil {
			conn.Close()
		}
	case *msg.NewVisitorConn:
		if err = svr.RegisterVisitorConn(conn, m); err != nil {
			xl.Warn("register visitor conn error: %v", err)
			_ = msg.WriteMsg(conn, &msg.NewVisitorConnResp{
				ProxyName: m.ProxyName,
				Error:     util.GenerateResponseErrorString("register visitor conn error", err, svr.Cfg.DetailedErrorsToClient),
			})
			conn.Close()
		} else {
			_ = msg.WriteMsg(conn, &msg.NewVisitorConnResp{
				ProxyName: m.ProxyName,
				Error:     "",
			})
		}
	default:
		log.Warn("Error message type for the new connection [%s]", conn.RemoteAddr().String())
		conn.Close()
	}
}

func (svr *Service) HandleListener(l net.Listener) {
	// Listen for incoming connections from client.
	for {
		c, err := l.Accept()
		if err != nil {
			log.Warn("Listener for incoming connections from client closed")
			return
		}
		// inject xlog object into net.Conn context
		xl := xlog.New()
		ctx := context.Background()

		c = frpNet.NewContextConn(xlog.NewContext(ctx, xl), c)

		log.Trace("start check TLS connection...")
		originConn := c
		var isTLS, custom bool
		c, isTLS, custom, err = frpNet.CheckAndEnableTLSServerConnWithTimeout(c, svr.tlsConfig, svr.Cfg.TLSOnly, connReadTimeout)
		if err != nil {
			log.Warn("CheckAndEnableTLSServerConnWithTimeout error: %v", err)
			originConn.Close()
			continue
		}
		log.Trace("check TLS connection success, isTLS: %v custom: %v", isTLS, custom)

		// Start a new goroutine to handle connection.
		go func(ctx context.Context, frpConn net.Conn) {
			if svr.Cfg.TCPMux {
				fmuxCfg := fmux.DefaultConfig()
				fmuxCfg.KeepAliveInterval = time.Duration(svr.Cfg.TCPMuxKeepaliveInterval) * time.Second
				fmuxCfg.LogOutput = io.Discard
				session, err := fmux.Server(frpConn, fmuxCfg)
				if err != nil {
					log.Warn("Failed to create mux connection: %v", err)
					frpConn.Close()
					return
				}

				for {
					stream, err := session.AcceptStream()
					if err != nil {
						log.Debug("Accept new mux stream error: %v", err)
						session.Close()
						return
					}
					go svr.handleConnection(ctx, stream)
				}
			} else {
				svr.handleConnection(ctx, frpConn)
			}
		}(ctx, c)
	}
}

func (svr *Service) RegisterControl(ctlConn net.Conn, LoginMsg *msg.Login) (err error) {
	// If client's RunID is empty, it's a new client, we just create a new controller.
	// Otherwise, we check if there is one controller has the same run id. If so, we release previous controller and start new one.
	if LoginMsg.RunID == "" {
		LoginMsg.RunID, err = util.RandID()
		if err != nil {
			return
		}
	}

	ctx := frpNet.NewContextFromConn(ctlConn)
	xl := xlog.FromContextSafe(ctx)
	xl.AppendPrefix(LoginMsg.RunID)
	ctx = xlog.NewContext(ctx, xl)
	xl.Info("client login info: ip [%s] version [%s] hostname [%s] os [%s] arch [%s]",
		ctlConn.RemoteAddr().String(), LoginMsg.Version, LoginMsg.Hostname, LoginMsg.Os, LoginMsg.Arch)

	// Check client version.
	if ok, msg := version.Compat(LoginMsg.Version); !ok {
		err = fmt.Errorf("%s", msg)
		return
	}

	// Check auth.
	if err = svr.authVerifier.VerifyLogin(LoginMsg); err != nil {
		return
	}

	ctl := NewControl(ctx, svr.rc, svr.PxyManager, svr.pluginManager, svr.authVerifier, ctlConn, LoginMsg, svr.Cfg)
	if oldCtl := svr.CtlManager.Add(LoginMsg.RunID, ctl); oldCtl != nil {
		oldCtl.allShutdown.WaitDone()
	}

	ctl.Start()

	// for statistics
	metrics.Server.NewClient()

	go func() {
		// block until control closed
		ctl.WaitClosed()
		svr.CtlManager.Del(LoginMsg.RunID, ctl)
	}()
	return
}

// RegisterWorkConn register a new work connection to control and proxies need it.
func (svr *Service) RegisterWorkConn(workConn net.Conn, newMsg *msg.NewWorkConn) error {
	xl := frpNet.NewLogFromConn(workConn)
	ctl, exist := svr.CtlManager.GetByID(newMsg.RunID)
	if !exist {
		xl.Warn("No client control found for run id [%s]", newMsg.RunID)
		return fmt.Errorf("no client control found for run id [%s]", newMsg.RunID)
	}
	// server plugin hook
	content := &plugin.NewWorkConnContent{
		User: plugin.UserInfo{
			User:  ctl.LoginMsg.User,
			Metas: ctl.LoginMsg.Metas,
			RunID: ctl.LoginMsg.RunID,
		},
		NewWorkConn: *newMsg,
	}
	retContent, err := svr.pluginManager.NewWorkConn(content)
	if err == nil {
		newMsg = &retContent.NewWorkConn
		// Check auth.
		err = svr.authVerifier.VerifyNewWorkConn(newMsg)
	}
	if err != nil {
		xl.Warn("invalid NewWorkConn with run id [%s]", newMsg.RunID)
		_ = msg.WriteMsg(workConn, &msg.StartWorkConn{
			Error: util.GenerateResponseErrorString("invalid NewWorkConn", err, ctl.serverCfg.DetailedErrorsToClient),
		})
		return fmt.Errorf("invalid NewWorkConn with run id [%s]", newMsg.RunID)
	}
	return ctl.RegisterWorkConn(workConn)
}

func (svr *Service) RegisterVisitorConn(visitorConn net.Conn, newMsg *msg.NewVisitorConn) error {
	return svr.rc.VisitorManager.NewConn(newMsg.ProxyName, visitorConn, newMsg.Timestamp, newMsg.SignKey,
		newMsg.UseEncryption, newMsg.UseCompression)
}
