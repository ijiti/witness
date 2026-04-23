// live.js — SSE live monitoring for witness.
// Uses htmx:afterSettle to detect content swaps and (re)init SSE connections.
// Note: innerHTML usage here is safe — content comes from server-rendered
// Go templates where all user content is escaped via template.HTMLEscapeString().
// skipcq: JS-0860 — innerHTML with trusted server content is intentional.

(function() {
    'use strict';

    // Active SSE connection — only one at a time (per-view stream).
    var activeSSE = null;
    // Pending project refresh timer — must be cancellable on navigation.
    var pendingRefresh = null;

    // Activity SSE — a single persistent connection for sidebar dots. Distinct
    // from per-view streams so it survives HTMX navigation. Keyed decay timers
    // clear each dot after ACTIVITY_DECAY_MS with no further writes; this must
    // stay loosely synced with ActivityWindow in internal/discovery/activity.go
    // so server-rendered initial state and client decay converge on the same
    // window.
    var activitySSE = null;
    var ACTIVITY_DECAY_MS = 30000;
    var projectTimers = {}; // projectID → timer clearing that project's dot
    var sessionTimers = {}; // "projectID/sessionID" → timer clearing that dot

    // Live-follow + scroll-position state for the session view.
    // Persisted across page loads via localStorage; default ON.
    var NEAR_BOTTOM_PX = 200;
    var liveFollow = (function() {
        try { return localStorage.getItem('witness:liveFollow') !== 'false'; }
        catch (e) { return true; }
    })();
    var unseenTurns = 0;
    var pillEl = null;
    var toggleEl = null;

    // Scroll-position tracking, throttled to one rAF per scroll event.
    var nearBottom = true;
    var scrollPending = false;
    function isNearBottom() {
        return (window.innerHeight + window.scrollY) >=
               (document.documentElement.scrollHeight - NEAR_BOTTOM_PX);
    }
    window.addEventListener('scroll', function() {
        if (scrollPending) { return; }
        scrollPending = true;
        requestAnimationFrame(function() {
            scrollPending = false;
            nearBottom = isNearBottom();
            if (nearBottom) { unseenTurns = 0; }
            updatePill();
        });
    }, { passive: true });

    function updatePill() {
        if (!pillEl) { return; }
        // Pill appears whenever the user has scrolled away from the bottom,
        // surfacing as a jump-to-latest affordance even when no new turns
        // have arrived yet. The label changes to reflect new-turn count.
        var visible = !nearBottom;
        pillEl.classList.toggle('hidden', !visible);
        var label = pillEl.querySelector('.pill-label');
        if (!label) { return; }
        label.textContent = unseenTurns > 0
            ? ('↓ ' + unseenTurns + ' new')
            : '↓ Latest';
    }

    function setLiveFollow(on) {
        liveFollow = !!on;
        try { localStorage.setItem('witness:liveFollow', liveFollow ? 'true' : 'false'); }
        catch (e) { /* private browsing — silently ignore */ }
        if (toggleEl) {
            toggleEl.setAttribute('aria-pressed', liveFollow ? 'true' : 'false');
            toggleEl.classList.toggle('text-green-400', liveFollow);
            toggleEl.classList.toggle('text-gray-500', !liveFollow);
        }
    }

    function scrollToBottom() {
        window.scrollTo({ top: document.documentElement.scrollHeight, behavior: 'smooth' });
        unseenTurns = 0;
        nearBottom = true; // pre-set to avoid pill flicker before scroll fires
        updatePill();
    }

    function bindLiveControls() {
        pillEl = document.getElementById('jump-to-latest');
        toggleEl = document.getElementById('live-follow-toggle');
        if (pillEl && !pillEl.dataset.bound) {
            pillEl.dataset.bound = '1';
            pillEl.addEventListener('click', scrollToBottom);
        }
        if (toggleEl && !toggleEl.dataset.bound) {
            toggleEl.dataset.bound = '1';
            toggleEl.addEventListener('click', function() { setLiveFollow(!liveFollow); });
        }
        setLiveFollow(liveFollow);
        unseenTurns = 0;
        nearBottom = isNearBottom();
        updatePill();
    }

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

    function setProjectActive(projectID) {
        if (!projectID) { return; }
        var dots = document.querySelectorAll(
            '[data-project-id="' + cssEscape(projectID) + '"] .activity-dot'
        );
        for (var i = 0; i < dots.length; i++) {
            dots[i].classList.remove('hidden');
        }
        if (projectTimers[projectID]) { clearTimeout(projectTimers[projectID]); }
        projectTimers[projectID] = setTimeout(function() {
            var again = document.querySelectorAll(
                '[data-project-id="' + cssEscape(projectID) + '"] .activity-dot'
            );
            for (var j = 0; j < again.length; j++) { again[j].classList.add('hidden'); }
            delete projectTimers[projectID];
        }, ACTIVITY_DECAY_MS);
    }

    function setSessionActive(projectID, sessionID) {
        if (!projectID || !sessionID) { return; }
        var dots = document.querySelectorAll(
            '[data-session-id="' + cssEscape(sessionID) +
            '"][data-session-project="' + cssEscape(projectID) + '"] .activity-dot'
        );
        for (var i = 0; i < dots.length; i++) {
            dots[i].classList.remove('hidden');
        }
        var key = projectID + '/' + sessionID;
        if (sessionTimers[key]) { clearTimeout(sessionTimers[key]); }
        sessionTimers[key] = setTimeout(function() {
            var again = document.querySelectorAll(
                '[data-session-id="' + cssEscape(sessionID) +
                '"][data-session-project="' + cssEscape(projectID) + '"] .activity-dot'
            );
            for (var j = 0; j < again.length; j++) { again[j].classList.add('hidden'); }
            delete sessionTimers[key];
        }, ACTIVITY_DECAY_MS);
    }

    // CSS.escape shim for selector safety — projectIDs are filesystem-derived
    // and can contain dots, quotes, etc. that break naive string concatenation.
    function cssEscape(s) {
        if (window.CSS && typeof window.CSS.escape === 'function') {
            return window.CSS.escape(s);
        }
        return String(s).replace(/([^a-zA-Z0-9_\-])/g, '\\$1');
    }

    function initActivitySSE() {
        if (activitySSE) { return; } // only one, survives navigation
        function connect() {
            var sse = new EventSource('/activity/stream');
            activitySSE = sse;
            sse.addEventListener('activity', function(e) {
                var payload;
                try { payload = JSON.parse(e.data); }
                catch (err) { return; }
                if (payload.projectID) { setProjectActive(payload.projectID); }
                if (payload.projectID && payload.sessionID) {
                    setSessionActive(payload.projectID, payload.sessionID);
                }
            });
            sse.onerror = function() {
                sse.close();
                if (activitySSE === sse) { activitySSE = null; }
                setTimeout(function() {
                    if (!activitySSE) { connect(); }
                }, 5000);
            };
        }
        connect();
    }

    function initSessionSSE(url) {
        var container = document.getElementById('turns-container');
        var header = document.getElementById('session-header');
        var indicator = document.getElementById('live-indicator');

        function setConnected(connected) {
            if (!indicator) { return; }
            var dot = indicator.querySelector('.live-status-dot');
            var txt = indicator.querySelector('.live-status-text');
            if (!dot || !txt) { return; }
            if (connected) {
                dot.className = 'live-status-dot w-2 h-2 rounded-full bg-green-500 animate-pulse';
                txt.textContent = 'live';
            } else {
                dot.className = 'live-status-dot w-2 h-2 rounded-full bg-yellow-500';
                txt.textContent = 'reconnecting...';
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
                div.innerHTML = e.data; // safe: server-rendered, HTML-escaped (see header)
                var added = 0;
                while (div.firstChild) {
                    container.appendChild(div.firstChild);
                    added++;
                }
                // Smart auto-scroll: follow only when the user is near the
                // bottom AND has live-follow enabled. Otherwise increment the
                // unseen-turn counter and surface the jump-to-latest pill.
                if (liveFollow && nearBottom) {
                    window.scrollTo(0, document.documentElement.scrollHeight);
                } else {
                    unseenTurns += added;
                    updatePill();
                }
            });

            sse.addEventListener('turn-update', function(e) {
                // Replace the last turn in the container (assistant streaming).
                if (!container) { return; }
                var lastChild = container.lastElementChild;
                if (!lastChild) { return; }
                var div = document.createElement('div');
                div.innerHTML = e.data; // safe: server-rendered, HTML-escaped (see header)
                var newTurn = div.firstElementChild;
                if (newTurn) {
                    container.replaceChild(newTurn, lastChild);
                }
                // If the user is following at the bottom, keep them pinned as
                // the in-flight turn grows. Otherwise leave the scroll alone.
                if (liveFollow && nearBottom) {
                    window.scrollTo(0, document.documentElement.scrollHeight);
                }
            });

            sse.addEventListener('header', function(e) {
                if (!header) { return; }
                header.innerHTML = e.data; // safe: server-rendered, HTML-escaped (see header)
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
            bindLiveControls();
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
    initActivitySSE(); // persistent sidebar dots, never closed
    initLive();

    // Re-init after every HTMX content swap (project/session navigation).
    // The sidebar and main content both re-render on full-page swaps, so
    // reapply any in-flight activity state to the new DOM elements.
    document.body.addEventListener('htmx:afterSettle', function(e) {
        if (e.detail.target && e.detail.target.id === 'main-content') {
            initLive();
            for (var pid in projectTimers) {
                if (Object.prototype.hasOwnProperty.call(projectTimers, pid)) {
                    var dots = document.querySelectorAll(
                        '[data-project-id="' + cssEscape(pid) + '"] .activity-dot'
                    );
                    for (var i = 0; i < dots.length; i++) { dots[i].classList.remove('hidden'); }
                }
            }
            for (var key in sessionTimers) {
                if (Object.prototype.hasOwnProperty.call(sessionTimers, key)) {
                    var parts = key.split('/');
                    if (parts.length !== 2) { continue; }
                    var sDots = document.querySelectorAll(
                        '[data-session-id="' + cssEscape(parts[1]) +
                        '"][data-session-project="' + cssEscape(parts[0]) + '"] .activity-dot'
                    );
                    for (var j = 0; j < sDots.length; j++) { sDots[j].classList.remove('hidden'); }
                }
            }
        }
    });
})();
