package diagnostics

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ErrLocalDNSBroken          = 1006
	ErrAPIDomainBlocked        = 1007
	ErrAPIIPBlockedOrDown      = 1008
	ErrVPSOutboundBlocked      = 1009
	ErrAPITLSInterference      = 1010
	ErrOpenVPNCmdNotFound      = 2001
	ErrOpenVPNStartFailed      = 2002
	ErrOpenVPNDNSResolve       = 2003
	ErrOpenVPNNodeUnreachable  = 2004
	ErrOpenVPNAuthFailed       = 2005
	ErrOpenVPNTLSBlocked       = 2006
	ErrOpenVPNPushOptions      = 2007
	ErrOpenVPNOptions          = 2008
	ErrOpenVPNTunUnavailable   = 2009
	ErrOpenVPNUnknown          = 2010
	ErrRouteForwardDisabled    = 3001
	ErrRouteRuleAddFailed      = 3002
	ErrRouteTableAddFailed     = 3003
	ErrRouteDevNotFound        = 3004
	ErrPortInUse               = 3005
	ErrProxyBindTunDenied      = 3006
	ErrFirewallBlockingForward = 3007
	ErrRouteRPFilterStrict     = 3008
)

const (
	TagLocalDNSBroken          = "ERR_LOCAL_DNS_BROKEN"
	TagAPIDomainBlocked        = "ERR_API_DOMAIN_BLOCKED"
	TagAPIIPBlockedOrDown      = "ERR_API_IP_BLOCKED_OR_DOWN"
	TagVPSOutboundBlocked      = "ERR_VPS_OUTBOUND_BLOCKED"
	TagAPITLSInterference      = "ERR_API_TLS_INTERFERENCE"
	TagOpenVPNCmdNotFound      = "ERR_OVPN_CMD_NOT_FOUND"
	TagOpenVPNStartFailed      = "ERR_OVPN_START_FAILED"
	TagOpenVPNDNSResolve       = "ERR_OVPN_DNS_RESOLVE"
	TagOpenVPNNodeUnreachable  = "ERR_OVPN_NODE_UNREACHABLE"
	TagOpenVPNAuthFailed       = "ERR_OVPN_AUTH_FAILED"
	TagOpenVPNTLSBlocked       = "ERR_OVPN_TLS_BLOCKED"
	TagOpenVPNPushOptions      = "ERR_OVPN_PUSH_OPTIONS"
	TagOpenVPNOptions          = "ERR_OVPN_OPTIONS"
	TagOpenVPNTunUnavailable   = "ERR_OVPN_TUN_NOT_AVAILABLE"
	TagOpenVPNUnknown          = "ERR_OVPN_UNKNOWN"
	TagRouteForwardDisabled    = "ERR_ROUTE_FORWARD_DISABLED"
	TagRouteRuleAddFailed      = "ERR_ROUTE_RULE_ADD_FAILED"
	TagRouteTableAddFailed     = "ERR_ROUTE_TABLE_ADD_FAILED"
	TagRouteDevNotFound        = "ERR_ROUTE_DEV_NOT_FOUND"
	TagPortInUse               = "ERR_PORT_IN_USE"
	TagProxyBindTunDenied      = "ERR_PROXY_BIND_TUN_PERM_DENIED"
	TagFirewallBlockingForward = "ERR_FIREWALL_BLOCKING_FORWARD"
	TagRouteRPFilterStrict     = "ERR_ROUTE_RP_FILTER_STRICT"
)

var (
	codePattern = regexp.MustCompile(`\[\d{4}\]`)
	codeByTag   = map[string]int{
		TagLocalDNSBroken:          ErrLocalDNSBroken,
		TagAPIDomainBlocked:        ErrAPIDomainBlocked,
		TagAPIIPBlockedOrDown:      ErrAPIIPBlockedOrDown,
		TagVPSOutboundBlocked:      ErrVPSOutboundBlocked,
		TagAPITLSInterference:      ErrAPITLSInterference,
		TagOpenVPNCmdNotFound:      ErrOpenVPNCmdNotFound,
		TagOpenVPNStartFailed:      ErrOpenVPNStartFailed,
		TagOpenVPNDNSResolve:       ErrOpenVPNDNSResolve,
		TagOpenVPNNodeUnreachable:  ErrOpenVPNNodeUnreachable,
		TagOpenVPNAuthFailed:       ErrOpenVPNAuthFailed,
		TagOpenVPNTLSBlocked:       ErrOpenVPNTLSBlocked,
		TagOpenVPNPushOptions:      ErrOpenVPNPushOptions,
		TagOpenVPNOptions:          ErrOpenVPNOptions,
		TagOpenVPNTunUnavailable:   ErrOpenVPNTunUnavailable,
		TagOpenVPNUnknown:          ErrOpenVPNUnknown,
		TagRouteForwardDisabled:    ErrRouteForwardDisabled,
		TagRouteRuleAddFailed:      ErrRouteRuleAddFailed,
		TagRouteTableAddFailed:     ErrRouteTableAddFailed,
		TagRouteDevNotFound:        ErrRouteDevNotFound,
		TagPortInUse:               ErrPortInUse,
		TagProxyBindTunDenied:      ErrProxyBindTunDenied,
		TagFirewallBlockingForward: ErrFirewallBlockingForward,
		TagRouteRPFilterStrict:     ErrRouteRPFilterStrict,
	}
)

func Format(code int, tag, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Sprintf("[%d] %s", code, tag)
	}
	return fmt.Sprintf("[%d] %s %s", code, tag, message)
}

func EnsureCode(message string) string {
	message = strings.TrimSpace(message)
	if message == "" || codePattern.MatchString(message) {
		return message
	}
	for tag, code := range codeByTag {
		if strings.Contains(message, tag) {
			clean := strings.Replace(message, "["+tag+"]", "", 1)
			clean = strings.Replace(clean, tag, "", 1)
			clean = strings.TrimSpace(strings.TrimPrefix(clean, ":"))
			return Format(code, tag, clean)
		}
	}
	return message
}
