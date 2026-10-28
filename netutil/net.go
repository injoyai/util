// Package netutil 提供 IP 地址解析及显式信任代理的客户端地址提取。
package netutil

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseV4 接受 IPv4 或 IPv4:port，省略端口时使用 defaultPort。
func ParseV4(s string, defaultPort uint16) (netip.AddrPort, error) {
	if a, err := netip.ParseAddr(s); err == nil && a.Is4() {
		return netip.AddrPortFrom(a, defaultPort), nil
	}
	a, err := netip.ParseAddrPort(s)
	if err != nil || !a.Addr().Is4() {
		return netip.AddrPort{}, fmt.Errorf("netutil: invalid IPv4 endpoint %q", s)
	}
	return a, nil
}

// PeerIP 只读取 RemoteAddr，不修改请求，不信任任何转发头。
func PeerIP(r *http.Request) (netip.Addr, error) {
	if r == nil {
		return netip.Addr{}, fmt.Errorf("netutil: nil request")
	}
	if a, err := netip.ParseAddr(r.RemoteAddr); err == nil {
		return a.Unmap(), nil
	}
	a, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Addr().Unmap(), nil
}

// ClientIP 仅当直接对端属于 trusted 时读取 X-Forwarded-For，
// 从右向左剥离受信代理，返回第一个不受信地址。无信任规则等价于 PeerIP。
// 上游代理必须按标准追加/清洗 X-Forwarded-For；不支持其他转发头。
func ClientIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, error) {
	peer, err := PeerIP(r)
	if err != nil {
		return netip.Addr{}, err
	}
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	if !isTrusted(peer) {
		return peer, nil
	}
	values := r.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return peer, nil
	}
	parts := strings.Split(strings.Join(values, ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return netip.Addr{}, fmt.Errorf("netutil: invalid forwarded address: %w", err)
		}
		peer = a.Unmap()
		if !isTrusted(peer) {
			return peer, nil
		}
	}
	return peer, nil
}

// LocalIPv4 返回已启用且非回环网卡的 IPv4 地址，传播网卡读取错误并去重。
func LocalIPv4() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []netip.Addr
	seen := make(map[netip.Addr]bool)
	for _, inter := range interfaces {
		if inter.Flags&net.FlagUp == 0 || inter.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := inter.Addrs()
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil {
				continue
			}
			a := prefix.Addr().Unmap()
			if a.Is4() && !a.IsLoopback() && !seen[a] {
				seen[a] = true
				result = append(result, a)
			}
		}
	}
	return result, nil
}
