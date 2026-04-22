// live.js — SSE live monitoring for witness.
// Uses htmx:afterSettle to detect content swaps and (re)init SSE connections.
// Note: innerHTML usage here is safe — content comes from server-rendered
// Go templates where all user content is escaped via template.HTMLEscapeString().
// skipcq: JS-0860 — innerHTML with trusted server content is intentional.

(function() {
    'use strict';

    // Active SSE connection — only one at a time.
    var activeSSE = null;
    // Pending project refresh timer — must be cancellable on navigation.
    var pendingRefresh = null;

    // Prevent subagent links inside details from toggling the parent.
    document.addEventListener('click', function(e) {
        var link = e.target.closest('.subagent-link');
        if (link) { e.stopPropagation(); }
    });

    function closeActive() {
        clearTimeout(pendingRefresh);
        pendingRefresh = null;
        if (activeSSE) {
            activeSSE.close();
            activeSSE = null;
        }
    }

    function initSessionSSE(url) {
        var container = document.getElementById('turns-container');
        var header = document.getElementById('session-header');
        var indicator = document.getElementById('live-indicator');

        function setConnected(connected) {
            if (!indicator) { return; }
            var dot = indicator.querySelector('span');
            if (connected) {
                dot.className = 'w-2 h-2 rounded-full bg-green-500 animate-pulse';
                indicator.lastChild.textContent = ' live';
            } else {
                dot.className = 'w-2 h-2 rounded-full bg-yellow-500';
                indicator.lastChild.textContent = ' reconnecting...';
            }
        }

        function connect() {
            closeActive();
            var sse = new EventSource(url);
            activeSSE = sse;
            setConnected(true);

            sse.addEventListener('turn', function(e) {
                if (!container) { return; }
                var div = document.createElement('div');
                div.innerHTML = e.data; // safe: server-rendered, HTML-escaped
                while (div.firstChild) {
                    container.appendChild(div.firstChild);
                }
                window.scrollTo(0, document.body.scrollHeight);
            });

            sse.addEventListener('turn-update', function(e) {
                // Replace the last turn in the container (assistant streaming).
                if (!container) { return; }
                var lastChild = container.lastElementChild;
                if (!lastChild) { return; }
                var div = document.createElement('div');
                div.innerHTML = e.data; // safe: server-rendered, HTML-escaped
                var newTurn = div.firstElementChild;
                if (newTurn) {
                    container.replaceChild(newTurn, lastChild);
                }
            });

            sse.addEventListener('header', function(e) {
                if (!header) { return; }
                header.innerHTML = e.data; // safe: server-rendered, HTML-escaped
            });

            sse.onerror = function() {
                setConnected(false);
                sse.close();
                if (activeSSE === sse) { activeSSE = null; }
                setTimeout(connect, 3000);
            };
        }

        connect();
    }

    function initProjectSSE(url, refreshUrl) {
        function connect() {
            closeActive();
            var sse = new EventSource(url);
            activeSSE = sse;

            sse.addEventListener('refresh', function() {
                if (!document.getElementById('live-project')) {
                    sse.close();
                    if (activeSSE === sse) { activeSSE = null; }
                    return;
                }
                clearTimeout(pendingRefresh);
                pendingRefresh = setTimeout(function() {
                    pendingRefresh = null;
                    if (document.getElementById('live-project')) {
                        htmx.ajax('GET', refreshUrl, {target: '#main-content'});
                    }
                }, 500);
            });

            sse.onerror = function() {
                sse.close();
                if (activeSSE === sse) { activeSSE = null; }
                setTimeout(connect, 5000);
            };
        }

        connect();
    }

    // Scan the DOM for SSE targets and start connections.
    function initLive() {
        // Session-level live updates.
        var sessionEl = document.getElementById('live-session');
        if (sessionEl) {
            var url = sessionEl.getAttribute('data-sse-url');
            if (url) { initSessionSSE(url); }
            return; // Session SSE takes priority.
        }

        // Project-level live updates (session list auto-refresh).
        var projectEl = document.getElementById('live-project');
        if (projectEl) {
            var purl = projectEl.getAttribute('data-sse-url');
            var refreshUrl = projectEl.getAttribute('data-refresh-url');
            if (purl && refreshUrl) { initProjectSSE(purl, refreshUrl); }
            return;
        }

        // No SSE target — clean up any stale connection.
        closeActive();
    }

    // Init on page load.
    initLive();

    // Re-init after every HTMX content swap (project/session navigation).
    document.body.addEventListener('htmx:afterSettle', function(e) {
        if (e.detail.target && e.detail.target.id === 'main-content') {
            initLive();
        }
    });
})();
