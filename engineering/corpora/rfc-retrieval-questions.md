# RFC retrieval question inventory

These 100 authored information needs are frozen before any retrieval or embedding scores. They are diagnostic questions, not production traffic. Each of ten topic families has nine evidence questions and one deployment-state control.

The split keeps six whole families (60 questions) for development and four whole families (40 questions) reserved from retrieval experiments and tuning. Protocols still overlap. The reserved set is not secret from the user or necessarily unknown to model training.

Independent review checked every question against exact captured source spans. Those source sections are not yet canonical search passages or exhaustive relevance judgments. The source column identifies known evidence; the [evaluation contract](../decisions/rfc-text-evaluation.md) explains explicit source requirements and the treatment of other useful results. A deployment-state control may have lexical matches.

Machine-readable questions, answers and source quotes remain private. Frozen question-set SHA-256: `d90b3ae5cb66acd4c34dddc6d5ac3dec91866451a1ddb38fc0d745807e41d994`.

## http-semantics (development)

| Intent | Question | Known source |
| --- | --- | --- |
| http-semantics-put-retry | A client sent a PUT but lost the connection before seeing the reply. When may it retry automatically? | RFC 9110 |
| http-semantics-safe-side-effects | Can a request method count as safe even if the server writes access logs or charges for advertising? | RFC 9110 |
| http-semantics-head-length | May a HEAD response contain Content-Length even though it has no response body? | RFC 9110 |
| http-semantics-unauthenticated-forbidden | How does HTTP distinguish missing valid authentication from a request the server understands but refuses? | RFC 9110 |
| http-semantics-redirect-method | Which redirect responses explicitly preserve the request method when following the new location? | RFC 9110 |
| http-semantics-etag-write-condition | When using If-Match to prevent overwriting someone else's changes, are weak entity tags sufficient? | RFC 9110 |
| http-semantics-conditional-not-modified | When If-None-Match prevents a request from proceeding, why can the response be 304 for a read but 412 for another method? | RFC 9110 |
| http-semantics-wrong-connection | A server says a request reached a connection it cannot serve. May the client retry on a different connection? | RFC 9110 |
| http-semantics-range-ignore | If a client asks for only a byte range, is the server always required to send a partial response? | RFC 9110 |
| http-semantics-deployment-control | What HTTP status did our production checkout service return to its most recent request? | Actual private deployment evidence required |

## http-caching (development)

| Intent | Question | Known source |
| --- | --- | --- |
| http-caching-revalidate-vs-discard | How can an origin require a cached response to be checked before reuse without prohibiting storage entirely? | RFC 9111 |
| http-caching-private-shared | What does marking a response private mean for a shared cache versus a browser's own cache? | RFC 9111 |
| http-caching-heuristic-freshness | Can a cache estimate freshness when the response lacks an explicit expiration time? | RFC 9111 |
| http-caching-vary-selection | Why can a cache need the original request headers to decide whether a stored response can satisfy a new request? | RFC 9111 |
| http-caching-authorized-shared | May a shared cache reuse a response to a request carrying Authorization for a later request? | RFC 9111 |
| http-caching-unsafe-invalidation | After a successful state-changing request, what cached resource entries must a cache invalidate? | RFC 9111 |
| http-caching-age-meaning | Does a response's Age value simply mean how long it sat in the last cache? | RFC 9111 |
| http-caching-stale-on-error | Can a cache serve stale content when an origin errors, and what limits the permitted staleness? | RFC 5861 |
| http-caching-warning-history | Comparing RFC 7234 with RFC 9111, what happened to the HTTP Warning header in the newer caching specification? | RFC 7234, RFC 9111 |
| http-caching-deployment-control | What freshness lifetime is currently configured for our company's production CDN? | Actual private deployment evidence required |

## http2-http3 (reserved)

| Intent | Question | Known source |
| --- | --- | --- |
| http2-http3-multiplex-streams | How can several HTTP/2 request-response exchanges share one connection without being the same stream? | RFC 9113 |
| http2-http3-flow-control-levels | In HTTP/2, does flow control apply to each stream, the whole connection, or both, and which frames consume it? | RFC 9113 |
| http2-http3-unknown-frames | Must an HTTP/2 endpoint fail the connection whenever it receives an unknown frame type? | RFC 9113 |
| http2-http3-stream-vs-connection-close | How do RST_STREAM and GOAWAY differ in the scope of what they terminate? | RFC 9113 |
| http2-http3-header-fragment-order | May an HTTP/2 sender interleave another stream's frames inside a fragmented header block? | RFC 9113 |
| http2-http3-frame-size-default | What is the default maximum HTTP/2 frame payload size before the peer advertises a different limit? | RFC 9113 |
| http2-http3-http3-transport | What transport does HTTP/3 use, and how are its requests associated with transport streams? | RFC 9114 |
| http2-http3-qpack-blocked-budget | How does a receiver limit the number of HTTP/3 streams that QPACK dynamic-table dependencies may block? | RFC 9204 |
| http2-http3-critical-stream-close | What happens if an HTTP/3 peer closes a critical control stream? | RFC 9114 |
| http2-http3-deployment-control | Is HTTP/3 currently enabled on our deployed edge gateway? | Actual private deployment evidence required |

## tls-pki (development)

| Intent | Question | Known source |
| --- | --- | --- |
| tls-pki-early-replay | Can a server treat TLS 1.3 early data as inherently protected against replay? | RFC 8446 |
| tls-pki-early-forward-secrecy | Does TLS 1.3 early data have the same forward-secrecy guarantees as ordinary 1-RTT data? | RFC 8446 |
| tls-pki-retry-handshake-twice | What must a TLS 1.3 client do if it receives a second HelloRetryRequest during the same connection attempt? | RFC 8446 |
| tls-pki-key-update | Can TLS 1.3 change application traffic keys without repeating the entire handshake? | RFC 8446 |
| tls-pki-signature-vs-finished | What different checks do CertificateVerify and Finished provide in TLS 1.3? | RFC 8446 |
| tls-pki-ticket-lifetime | For how long may a TLS 1.3 server advertise a session ticket as valid? | RFC 8446 |
| tls-pki-version-field | Why does a TLS 1.3 ClientHello carry a legacy version value instead of putting TLS 1.3 directly in that field? | RFC 8446 |
| tls-pki-sequence-wrap | May a TLS record sequence number wrap around while continuing to use the same traffic keys? | RFC 8446 |
| tls-pki-certificate-common-name | Under RFC 9525, can a certificate's Common Name substitute for the required service identity in subjectAltName? | RFC 9525 |
| tls-pki-deployment-control | Which cipher suite did our production API negotiate in its latest TLS connection? | Actual private deployment evidence required |

## dns (development)

| Intent | Question | Known source |
| --- | --- | --- |
| dns-recursive-vs-iterative | What work does a DNS server promise when it accepts a recursive query, compared with an iterative referral? | RFC 1034 |
| dns-negative-answer-types | How does a resolver distinguish a nonexistent domain from an existing name that lacks the requested record type? | RFC 2308 |
| dns-negative-cache-time | Which SOA values bound how long a negative DNS answer can be cached? | RFC 2308 |
| dns-truncated-response | If a DNS reply is too large for UDP and marked truncated, what should the client do? | RFC 7766 |
| dns-dnssec-confidentiality | Does DNSSEC hide DNS questions and answers from observers? | RFC 4033 |
| dns-alias-data | Why should an ordinary address record not coexist with a CNAME at the same owner name? | RFC 1034 |
| dns-doh-get-encoding | When carrying a DNS query in an HTTPS GET URL, how is the DNS message encoded? | RFC 8484 |
| dns-dot-port | Which port is designated for DNS over TLS, and may an unencrypted DNS exchange use that same connection? | RFC 7858 |
| dns-authenticated-denial | How can a validating resolver verify a claim that a DNS name or record does not exist? | RFC 4035 |
| dns-deployment-control | What records are currently stored in our office resolver's live DNS cache? | Actual private deployment evidence required |

## tcp (development)

| Intent | Question | Known source |
| --- | --- | --- |
| tcp-send-window | How do the peer's receive window and the sender's congestion window jointly limit TCP transmission? | RFC 5681 |
| tcp-ack-number | Does a TCP acknowledgment number report the last byte received, or the next sequence number expected? | RFC 9293 |
| tcp-zero-window-probe | How can a TCP sender discover that a closed receive window has reopened if the window update was lost? | RFC 9293 |
| tcp-time-wait | Why does TCP keep a TIME-WAIT state after active close, and how long is the specified wait? | RFC 9293 |
| tcp-rto-backoff | After a TCP retransmission timer expires repeatedly, how is the timeout adjusted? | RFC 6298 |
| tcp-initial-rto | Under RFC 6298, what initial retransmission timeout is recommended before any round-trip measurement exists? | RFC 6298 |
| tcp-ambiguous-rtt | Why should a TCP sender normally avoid measuring round-trip time from an acknowledgment of retransmitted data? | RFC 6298 |
| tcp-fast-retransmit | How can TCP infer a likely lost segment before its retransmission timer expires? | RFC 5681 |
| tcp-sack-vs-ack | What does the TCP SACK option add beyond the cumulative acknowledgment number? | RFC 2018 |
| tcp-deployment-control | What is the current congestion-window size of the TCP connection open on my laptop? | Actual private deployment evidence required |

## quic (reserved)

| Intent | Question | Known source |
| --- | --- | --- |
| quic-independent-streams | Why can data loss on one QUIC stream avoid blocking delivery of another stream's data? | RFC 9000 |
| quic-packet-number-reuse | When QUIC retransmits lost data, does it reuse the lost packet's number? | RFC 9000 |
| quic-migration-identity | How can a QUIC connection survive a client address change without treating it as a new connection? | RFC 9000 |
| quic-initial-datagram | What minimum UDP payload size must a client's QUIC Initial datagram reach? | RFC 9000 |
| quic-amplification-budget | Before validating a peer address, how much response traffic may a QUIC server send relative to received traffic? | RFC 9000 |
| quic-two-flow-control-budgets | How do QUIC's connection-wide and per-stream flow-control limits differ? | RFC 9000 |
| quic-unreliable-messages | Can an application send unreliable messages over QUIC without placing them on a reliable ordered stream? | RFC 9221 |
| quic-packet-loss-threshold | In QUIC loss detection, what packet-number gap is the recommended packet threshold? | RFC 9002 |
| quic-probe-timeout | Does expiration of QUIC's probe timeout itself mean every outstanding packet should be declared lost? | RFC 9002 |
| quic-deployment-control | Which network path is our production QUIC connection currently using? | Actual private deployment evidence required |

## oauth-jwt (reserved)

| Intent | Question | Known source |
| --- | --- | --- |
| oauth-jwt-delegation | What does OAuth 2.0 let an application obtain without giving it the resource owner's password? | RFC 6749 |
| oauth-jwt-redirect-matching | Under RFC 9700, how strictly must an authorization server match registered redirect URIs? | RFC 9700 |
| oauth-jwt-implicit-risk | Why does RFC 9700 favour a code-based flow over delivering access tokens in authorization responses? | RFC 9700 |
| oauth-jwt-pkce-interception | How does PKCE make an intercepted authorization code insufficient for redeeming a token? | RFC 7636 |
| oauth-jwt-bearer-possession | What makes a bearer access token different from a token requiring proof of possession of a key? | RFC 6750 |
| oauth-jwt-jwt-audience | Which validation prevents a JWT issued for one application from being accepted by another? | RFC 8725 |
| oauth-jwt-signed-readable | Does signing a JWT make its claims confidential? | RFC 7519 |
| oauth-jwt-algorithm-confusion | Why must a JWT consumer restrict acceptable cryptographic algorithms instead of trusting the token's alg header alone? | RFC 8725 |
| oauth-jwt-password-grant | Under RFC 9700, should a new OAuth client use the resource-owner password credentials grant? | RFC 9700 |
| oauth-jwt-deployment-control | What access-token lifetime is currently configured for our application's authorization server? | Actual private deployment evidence required |

## formats (development)

| Intent | Question | Known source |
| --- | --- | --- |
| formats-reserved-decoding | Can decoding every percent-encoded character before parsing a URI change its meaning? | RFC 3986 |
| formats-fragment-network | Is the fragment part of a URI sent to the server as part of dereferencing the resource? | RFC 3986 |
| formats-relative-resolution | How are dot and double-dot path segments handled when resolving a relative URI against a base? | RFC 3986 |
| formats-duplicate-json-names | What interoperability problem arises when a JSON object contains the same member name twice? | RFC 8259 |
| formats-json-integer-range | Which integer range can common IEEE 754 binary64 implementations agree on exactly when exchanging JSON? | RFC 8259 |
| formats-json-order | Should software rely on JSON object member ordering being preserved by every parser? | RFC 8259 |
| formats-json-wire-encoding | What text encoding is required for JSON exchanged between systems outside a closed ecosystem? | RFC 8259 |
| formats-surrogate-json | Can JSON syntax admit a lone UTF-16 surrogate even though implementations may disagree about it? | RFC 8259 |
| formats-overlong-utf8 | Why must a UTF-8 decoder protect against decoding overlong encodings as alternate spellings? | RFC 3629 |
| formats-deployment-control | Does our deployed application's JSON serializer preserve object member insertion order? | Actual private deployment evidence required |

## email (reserved)

| Intent | Question | Known source |
| --- | --- | --- |
| email-envelope-header | How does the SMTP envelope sender differ from the From field visible inside an email message? | RFC 5321, RFC 5322 |
| email-bcc-delivery | How can a blind-copy recipient receive email without appearing in the message's visible recipient headers? | RFC 5321, RFC 5322 |
| email-smtp-retry | How should an SMTP client treat a 4xx response differently from a 5xx response? | RFC 5321 |
| email-dot-stuffing | How does SMTP transmit a message body line beginning with a period without accidentally ending DATA? | RFC 5321 |
| email-mime-binary | How does MIME Base64 allow arbitrary binary body data to travel through a 7-bit mail transport? | RFC 2045 |
| email-multipart-boundary | How does a MIME receiver know where one multipart body part ends and the next begins? | RFC 2046 |
| email-spf-identity | Does an SPF pass directly authenticate the human-readable From header shown by a mail client? | RFC 7208 |
| email-dkim-identity | Does a valid DKIM signature identify an individual author, or a signing domain taking responsibility for a message? | RFC 6376 |
| email-dmarc-alignment | According to RFC 7489, must both SPF and DKIM pass for DMARC to pass, or can one aligned result be enough? | RFC 7489 |
| email-deployment-control | What spam score did our deployed mail gateway assign to the most recently delivered message? | Actual private deployment evidence required |
