use std::collections::HashSet;
use std::io;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};

use reqwest::redirect::Policy;

#[derive(Clone)]
pub(crate) enum OutboundNetworkPolicy {
    PublicOnly,
    /// Allows RFC1918/unique-local targets only for exact, control-plane
    /// configured hostnames. Loopback, link-local, metadata, and every other
    /// special-purpose address remain blocked.
    AllowTrustedPrivateHosts(HashSet<String>),
    AllowLoopbackForTests,
}

impl OutboundNetworkPolicy {
    pub(crate) fn public_with_trusted_private_hosts(hosts: HashSet<String>) -> Self {
        if hosts.is_empty() {
            Self::PublicOnly
        } else {
            Self::AllowTrustedPrivateHosts(hosts)
        }
    }

    fn allows_trusted_private_host(&self, host: &str) -> bool {
        matches!(self, Self::AllowTrustedPrivateHosts(hosts) if hosts.contains(host))
    }
}

pub(crate) struct PinnedHttpTarget {
    pub(crate) client: reqwest::Client,
    pub(crate) url: reqwest::Url,
}

#[derive(Debug, thiserror::Error)]
pub(crate) enum OutboundTargetError {
    #[error("invalid MCP URL: {0}")]
    InvalidUrl(#[from] url::ParseError),
    #[error("MCP URL must use HTTPS")]
    InsecureScheme,
    #[error("MCP URL must not contain credentials or a fragment")]
    DisallowedUrlComponent,
    #[error("MCP URL is missing a host or known port")]
    MissingAuthority,
    #[error("failed to resolve MCP URL host: {0}")]
    Resolution(#[source] io::Error),
    #[error("MCP URL host resolved to no addresses")]
    NoAddresses,
    #[error("MCP URL target {0} is blocked by outbound network policy")]
    BlockedAddress(IpAddr),
    #[error("failed to create hardened MCP HTTP client: {0}")]
    Client(#[from] reqwest::Error),
}

/// Resolves a target exactly once, validates every answer, and pins those
/// answers into a redirect- and proxy-free HTTP client. Keeping the original
/// hostname in the URL preserves the Host header and TLS SNI while preventing
/// a second DNS lookup from changing the dial destination.
pub(crate) async fn prepare_http_target(
    raw_url: &str,
    policy: OutboundNetworkPolicy,
) -> Result<PinnedHttpTarget, OutboundTargetError> {
    let url = reqwest::Url::parse(raw_url)?;
    if !url.username().is_empty() || url.password().is_some() || url.fragment().is_some() {
        return Err(OutboundTargetError::DisallowedUrlComponent);
    }
    if url.scheme() != "https"
        && !(matches!(policy, OutboundNetworkPolicy::AllowLoopbackForTests)
            && url.scheme() == "http")
    {
        return Err(OutboundTargetError::InsecureScheme);
    }

    let host = url
        .host_str()
        .ok_or(OutboundTargetError::MissingAuthority)?
        .trim_end_matches('.')
        .to_ascii_lowercase();
    let port = url
        .port_or_known_default()
        .ok_or(OutboundTargetError::MissingAuthority)?;
    let addresses = resolve_once(&host, port).await?;
    validate_addresses(&host, &addresses, &policy)?;

    // Plain HTTP exists solely for loopback integration fixtures. Even the
    // injected test policy must never allow cleartext traffic to a public or
    // private non-loopback destination.
    if url.scheme() == "http" && !addresses.iter().all(|address| address.ip().is_loopback()) {
        return Err(OutboundTargetError::InsecureScheme);
    }

    let client = reqwest::Client::builder()
        .no_proxy()
        .redirect(Policy::none())
        .referer(false)
        .pool_max_idle_per_host(0)
        .resolve_to_addrs(&host, &addresses)
        .build()?;

    Ok(PinnedHttpTarget { client, url })
}

async fn resolve_once(host: &str, port: u16) -> Result<Vec<SocketAddr>, OutboundTargetError> {
    let mut addresses = if let Ok(ip) = host.parse::<IpAddr>() {
        vec![SocketAddr::new(ip, port)]
    } else {
        tokio::net::lookup_host((host, port))
            .await
            .map_err(OutboundTargetError::Resolution)?
            .collect::<Vec<_>>()
    };
    let mut seen = HashSet::new();
    addresses.retain(|address| seen.insert(*address));
    if addresses.is_empty() {
        return Err(OutboundTargetError::NoAddresses);
    }
    Ok(addresses)
}

fn validate_addresses(
    host: &str,
    addresses: &[SocketAddr],
    policy: &OutboundNetworkPolicy,
) -> Result<(), OutboundTargetError> {
    for address in addresses {
        let ip = address.ip();
        let loopback_test_exception =
            matches!(policy, OutboundNetworkPolicy::AllowLoopbackForTests) && ip.is_loopback();
        let trusted_private_host_exception =
            policy.allows_trusted_private_host(host) && is_routable_private_ip(ip);
        if is_forbidden_ip(ip) && !loopback_test_exception && !trusted_private_host_exception {
            return Err(OutboundTargetError::BlockedAddress(ip));
        }
    }
    Ok(())
}

fn is_forbidden_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(address) => is_forbidden_ipv4(address),
        IpAddr::V6(address) => is_forbidden_ipv6(address),
    }
}

fn is_routable_private_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(address) => {
            ipv4_in_prefix(address, Ipv4Addr::new(10, 0, 0, 0), 8)
                || ipv4_in_prefix(address, Ipv4Addr::new(172, 16, 0, 0), 12)
                || ipv4_in_prefix(address, Ipv4Addr::new(192, 168, 0, 0), 16)
        }
        IpAddr::V6(address) => address
            .to_ipv4_mapped()
            .map(|mapped| is_routable_private_ip(IpAddr::V4(mapped)))
            .unwrap_or_else(|| ipv6_in_prefix(address, ipv6([0xfc00, 0, 0, 0, 0, 0, 0, 0]), 7)),
    }
}

fn is_forbidden_ipv4(address: Ipv4Addr) -> bool {
    const BLOCKED: &[(Ipv4Addr, u8)] = &[
        (Ipv4Addr::new(0, 0, 0, 0), 8),       // current network / unspecified
        (Ipv4Addr::new(10, 0, 0, 0), 8),      // private
        (Ipv4Addr::new(100, 64, 0, 0), 10),   // carrier-grade NAT
        (Ipv4Addr::new(127, 0, 0, 0), 8),     // loopback
        (Ipv4Addr::new(169, 254, 0, 0), 16),  // link-local and cloud metadata
        (Ipv4Addr::new(172, 16, 0, 0), 12),   // private
        (Ipv4Addr::new(192, 0, 0, 0), 24),    // IETF protocol assignments
        (Ipv4Addr::new(192, 0, 2, 0), 24),    // documentation
        (Ipv4Addr::new(192, 88, 99, 0), 24),  // deprecated 6to4 relay anycast
        (Ipv4Addr::new(192, 168, 0, 0), 16),  // private
        (Ipv4Addr::new(198, 18, 0, 0), 15),   // benchmark testing
        (Ipv4Addr::new(198, 51, 100, 0), 24), // documentation
        (Ipv4Addr::new(203, 0, 113, 0), 24),  // documentation
        (Ipv4Addr::new(224, 0, 0, 0), 4),     // multicast
        (Ipv4Addr::new(240, 0, 0, 0), 4),     // reserved and broadcast
    ];
    BLOCKED
        .iter()
        .any(|(network, prefix)| ipv4_in_prefix(address, *network, *prefix))
}

fn is_forbidden_ipv6(address: Ipv6Addr) -> bool {
    if let Some(mapped) = address.to_ipv4_mapped() {
        return is_forbidden_ipv4(mapped);
    }

    const BLOCKED: &[(Ipv6Addr, u8)] = &[
        (Ipv6Addr::UNSPECIFIED, 96), // unspecified and IPv4-compatible space
        (Ipv6Addr::LOCALHOST, 128),  // loopback
        (ipv6([0x0064, 0xff9b, 0, 0, 0, 0, 0, 0]), 96), // NAT64 well-known prefix
        (ipv6([0x0064, 0xff9b, 1, 0, 0, 0, 0, 0]), 48), // local-use NAT64
        (ipv6([0x0100, 0, 0, 0, 0, 0, 0, 0]), 64), // discard-only
        (ipv6([0x2001, 0, 0, 0, 0, 0, 0, 0]), 23), // IETF special-purpose range
        (ipv6([0x2001, 0x0db8, 0, 0, 0, 0, 0, 0]), 32), // documentation
        (ipv6([0x2002, 0, 0, 0, 0, 0, 0, 0]), 16), // deprecated 6to4
        (ipv6([0x3fff, 0, 0, 0, 0, 0, 0, 0]), 20), // documentation
        (ipv6([0x5f00, 0, 0, 0, 0, 0, 0, 0]), 16), // segment-routing SIDs
        (ipv6([0xfc00, 0, 0, 0, 0, 0, 0, 0]), 7), // unique-local
        (ipv6([0xfe80, 0, 0, 0, 0, 0, 0, 0]), 10), // link-local
        (ipv6([0xfec0, 0, 0, 0, 0, 0, 0, 0]), 10), // deprecated site-local
        (ipv6([0xff00, 0, 0, 0, 0, 0, 0, 0]), 8), // multicast
    ];
    BLOCKED
        .iter()
        .any(|(network, prefix)| ipv6_in_prefix(address, *network, *prefix))
}

const fn ipv6(segments: [u16; 8]) -> Ipv6Addr {
    Ipv6Addr::new(
        segments[0],
        segments[1],
        segments[2],
        segments[3],
        segments[4],
        segments[5],
        segments[6],
        segments[7],
    )
}

fn ipv4_in_prefix(address: Ipv4Addr, network: Ipv4Addr, prefix: u8) -> bool {
    let mask = u32::MAX << (32 - prefix);
    u32::from(address) & mask == u32::from(network) & mask
}

fn ipv6_in_prefix(address: Ipv6Addr, network: Ipv6Addr, prefix: u8) -> bool {
    let mask = u128::MAX << (128 - prefix);
    u128::from(address) & mask == u128::from(network) & mask
}

#[cfg(test)]
mod tests {
    use super::{is_forbidden_ip, validate_addresses, OutboundNetworkPolicy, OutboundTargetError};
    use std::collections::HashSet;
    use std::net::{IpAddr, SocketAddr};

    #[test]
    fn blocks_non_public_ipv4_ranges() {
        for value in [
            "0.0.0.0",
            "10.1.2.3",
            "100.64.0.1",
            "127.0.0.1",
            "169.254.169.254",
            "172.31.0.1",
            "192.168.1.1",
            "198.18.0.1",
            "224.0.0.1",
            "255.255.255.255",
        ] {
            assert!(is_forbidden_ip(value.parse().unwrap()), "allowed {value}");
        }
        assert!(!is_forbidden_ip("8.8.8.8".parse().unwrap()));
    }

    #[test]
    fn blocks_non_public_ipv6_and_embedded_ipv4_ranges() {
        for value in [
            "::",
            "::1",
            "::ffff:127.0.0.1",
            "64:ff9b::7f00:1",
            "100::1",
            "2001:db8::1",
            "fc00::1",
            "fe80::1",
            "ff02::1",
        ] {
            assert!(is_forbidden_ip(value.parse().unwrap()), "allowed {value}");
        }
        assert!(!is_forbidden_ip("2606:4700:4700::1111".parse().unwrap()));
    }

    #[test]
    fn rejects_all_answers_when_dns_contains_one_blocked_address() {
        let addresses = [
            SocketAddr::new(IpAddr::V4("8.8.8.8".parse().unwrap()), 443),
            SocketAddr::new(IpAddr::V4("127.0.0.1".parse().unwrap()), 443),
        ];
        assert!(matches!(
            validate_addresses(
                "mcp.example.com",
                &addresses,
                &OutboundNetworkPolicy::PublicOnly
            ),
            Err(OutboundTargetError::BlockedAddress(_))
        ));
    }

    #[test]
    fn test_policy_only_relaxes_loopback() {
        let loopback = [SocketAddr::new(
            IpAddr::V4("127.0.0.1".parse().unwrap()),
            8080,
        )];
        assert!(validate_addresses(
            "localhost",
            &loopback,
            &OutboundNetworkPolicy::AllowLoopbackForTests,
        )
        .is_ok());

        let metadata = [SocketAddr::new(
            IpAddr::V4("169.254.169.254".parse().unwrap()),
            80,
        )];
        assert!(validate_addresses(
            "metadata.example.com",
            &metadata,
            &OutboundNetworkPolicy::AllowLoopbackForTests,
        )
        .is_err());
    }

    #[test]
    fn allows_private_addresses_only_for_trusted_exact_hosts() {
        let private_runner = [SocketAddr::new(
            IpAddr::V4("10.80.1.3".parse().unwrap()),
            443,
        )];
        let trusted_hosts = HashSet::from(["staging.mcp.usehivy.com".to_string()]);
        let policy = OutboundNetworkPolicy::public_with_trusted_private_hosts(trusted_hosts);

        assert!(validate_addresses("staging.mcp.usehivy.com", &private_runner, &policy).is_ok());
        assert!(matches!(
            validate_addresses("untrusted.example.com", &private_runner, &policy),
            Err(OutboundTargetError::BlockedAddress(_))
        ));
    }

    #[test]
    fn trusted_hosts_do_not_bypass_metadata_or_loopback_protection() {
        let trusted_hosts = HashSet::from(["staging.mcp.usehivy.com".to_string()]);
        let policy = OutboundNetworkPolicy::public_with_trusted_private_hosts(trusted_hosts);
        for blocked in ["127.0.0.1", "169.254.169.254"] {
            let addresses = [SocketAddr::new(blocked.parse().unwrap(), 443)];
            assert!(matches!(
                validate_addresses("staging.mcp.usehivy.com", &addresses, &policy),
                Err(OutboundTargetError::BlockedAddress(_))
            ));
        }
    }
}
