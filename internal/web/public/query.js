"use strict";

(function () {
    var apiBase = "";
    var serversData = null;
    var mastersData = null;
    var sortState = {
        servers: { key: "num-players", asc: false },
        masters: { key: "address", asc: true }
    };
    var selectedServer = null; // address string
    var selectedMaster = null; // address string
    var playerSortState = { col: -1, asc: true }; // col index into expanded player columns
    var teamSortState = { col: -1, asc: true }; // col index into expanded team columns
    var refreshInterval = null;
    var autoRefreshEnabled = true;

    var refreshMs = 15000;

    // ===== Theme =====
    function getEffectiveTheme() {
        var stored = localStorage.getItem("t1ms-theme");
        if (stored === "light" || stored === "dark") return stored;
        if (window.matchMedia && window.matchMedia("(prefers-color-scheme: light)").matches) return "light";
        return "dark";
    }

    function applyTheme(theme) {
        document.documentElement.setAttribute("data-theme", theme);
        var btn = document.getElementById("themeToggle");
        if (btn) btn.textContent = theme === "dark" ? "Light" : "Dark";
    }

    window.toggleTheme = function () {
        var current = document.documentElement.getAttribute("data-theme") || "dark";
        var next = current === "dark" ? "light" : "dark";
        localStorage.setItem("t1ms-theme", next);
        applyTheme(next);
    };

    // ===== Init =====
    function init() {
        applyTheme(getEffectiveTheme());
        document.querySelectorAll("th[data-sort]").forEach(function (th) {
            th.addEventListener("click", function () {
                var tab = th.closest(".panel-content").id.replace("tab-", "");
                handleSort(tab, th.getAttribute("data-sort"));
            });
        });
        fetchAll();
        startAutoRefresh();
    }

    function startAutoRefresh() {
        if (refreshInterval) clearInterval(refreshInterval);
        if (!autoRefreshEnabled) return;
        refreshInterval = setInterval(fetchAll, refreshMs);
    }

    window.toggleAutoRefresh = function () {
        autoRefreshEnabled = !autoRefreshEnabled;
        var el = document.getElementById("refreshToggle");
        var label = document.getElementById("refreshLabel");
        if (autoRefreshEnabled) {
            el.classList.remove("paused");
            label.textContent = "AUTO";
            startAutoRefresh();
        } else {
            el.classList.add("paused");
            label.textContent = "PAUSED";
            if (refreshInterval) { clearInterval(refreshInterval); refreshInterval = null; }
        }
    };

    window.applyApiBase = function () {
        var val = document.getElementById("apiBase").value.replace(/\/+$/, "").trim();
        if (val && !/^https?:\/\//i.test(val)) {
            val = "https://" + val;
        }
        apiBase = val;
        fetchAll();
    };

    window.switchTab = function (tab) {
        document.querySelectorAll(".tab").forEach(function (t) {
            t.classList.toggle("active", t.getAttribute("data-tab") === tab);
        });
        document.querySelectorAll(".panel-content").forEach(function (p) {
            p.classList.toggle("active", p.id === "tab-" + tab);
        });
    };

    // ===== Fetch =====
    function fetchAll() {
        fetchServers();
        fetchMasters();
    }

    function fetchServers() {
        fetch(apiBase + "/api/v1/servers.json")
            .then(function (r) {
                if (!r.ok) throw new Error(r.status + " " + r.statusText);
                return r.json();
            })
            .then(function (data) {
                serversData = data;
                updateStats(data);
                updateMotd(data);
                renderServers();
                renderServerDetail();
            })
            .catch(function (err) {
                showError("serversLoading", "serversContent", err.message);
            });
    }

    function fetchMasters() {
        fetch(apiBase + "/api/v1/masters.json")
            .then(function (r) {
                if (!r.ok) throw new Error(r.status + " " + r.statusText);
                return r.json();
            })
            .then(function (data) {
                mastersData = data;
                renderMasters();
                renderMasterDetail();
                renderSetup();
            })
            .catch(function (err) {
                showError("mastersLoading", "mastersContent", err.message);
            });
    }

    function showError(loadingId, contentId, msg) {
        var el = document.getElementById(loadingId);
        el.textContent = "Error: " + msg;
        el.className = "status-msg error";
        el.style.display = "";
        document.getElementById(contentId).style.display = "none";
    }

    // ===== Stats & MOTD =====
    function updateStats(data) {
        document.getElementById("statServers").textContent = data["total-servers"] || 0;
        document.getElementById("statPlayers").textContent = data["total-players"] || 0;
        document.getElementById("statMaxPlayers").textContent = data["total-max-players"] || 0;
        document.getElementById("statClients").textContent = data["unique-clients"] || 0;

        var interval = data["refresh-interval"];
        if (interval && interval > 0) {
            var newMs = interval * 1000;
            if (newMs !== refreshMs) {
                refreshMs = newMs;
                startAutoRefresh();
            }
        }
    }

    function updateMotd(data) {
        var motd = data["message-of-the-day"] || "";
        var clean = motd.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
        document.getElementById("motd").textContent = clean ? "// " + clean : "";
    }

    // ===== Sorting =====
    function handleSort(tab, key) {
        var state = sortState[tab];
        if (state.key === key) {
            state.asc = !state.asc;
        } else {
            state.key = key;
            state.asc = true;
        }
        if (tab === "servers") renderServers();
        else renderMasters();
    }

    function sortArray(arr, key, asc) {
        return arr.slice().sort(function (a, b) {
            var va = a[key], vb = b[key];
            if (va == null) va = "";
            if (vb == null) vb = "";
            if (typeof va === "number" && typeof vb === "number") {
                return asc ? va - vb : vb - va;
            }
            if (typeof va === "boolean" && typeof vb === "boolean") {
                return asc ? (va === vb ? 0 : va ? -1 : 1) : (va === vb ? 0 : va ? 1 : -1);
            }
            va = String(va).toLowerCase();
            vb = String(vb).toLowerCase();
            if (va < vb) return asc ? -1 : 1;
            if (va > vb) return asc ? 1 : -1;
            return 0;
        });
    }

    function updateSortHeaders(tab) {
        var state = sortState[tab];
        var panel = document.getElementById("tab-" + tab);
        panel.querySelectorAll("th[data-sort]").forEach(function (th) {
            th.classList.remove("sorted-asc", "sorted-desc");
            if (th.getAttribute("data-sort") === state.key) {
                th.classList.add(state.asc ? "sorted-asc" : "sorted-desc");
            }
        });
    }

    // ===== Render Servers List =====
    function renderServers() {
        if (!serversData || !serversData.servers) {
            if (serversData && serversData["total-servers"] === 0) {
                showEmpty("serversLoading", "serversContent", "No servers online");
            }
            return;
        }
        var servers = serversData.servers;
        if (!servers.length) {
            showEmpty("serversLoading", "serversContent", "No servers online");
            return;
        }

        var sorted = sortArray(servers, sortState.servers.key, sortState.servers.asc);
        updateSortHeaders("servers");

        var html = "";
        for (var i = 0; i < sorted.length; i++) {
            var s = sorted[i];
            var np = s["num-players"] || 0;
            var mp = s["max-players"] || 0;
            var playerClass = np === 0 ? "players-empty" : (np >= mp ? "players-full" : "players-active");
            var ded = s.dedicated ? '<span class="icon-dedicated" title="Dedicated">&#x1F5A5;</span>' : '';
            var pw = s.password ? '<span class="icon-lock" title="Password">&#x1F5DD;</span>' : '';
            var sel = (s.address === selectedServer) ? " row-selected" : "";

            html += '<tr class="clickable' + sel + '" onclick="selectServer(\'' + escAttr(s.address) + '\')">';
            html += '<td class="text-center">' + ded + pw + '</td>';
            html += '<td>' + esc(s.name) + '</td>';
            html += '<td class="text-right ' + playerClass + '">' + np + '/' + mp + '</td>';
            html += '<td>' + esc(s.mission) + '</td>';
            html += '<td>' + esc(s["server-type"]) + '</td>';
            html += '<td>' + esc(s.mod) + '</td>';
            html += '<td class="text-right">' + (s.ping || 0) + '</td>';
            html += '<td>' + esc(s.address) + '</td>';
            html += '</tr>';
        }

        document.getElementById("serversBody").innerHTML = html;
        document.getElementById("serversLoading").style.display = "none";
        document.getElementById("serversContent").style.display = "";
    }

    window.selectServer = function (address) {
        var newAddr = (selectedServer === address) ? null : address;
        if (newAddr !== selectedServer) {
            playerSortState.col = -1;
            playerSortState.asc = true;
            teamSortState.col = -1;
            teamSortState.asc = true;
        }
        selectedServer = newAddr;
        renderServers();
        renderServerDetail(true);
    };

    window.goToServer = function (address) {
        if (address !== selectedServer) {
            playerSortState.col = -1;
            playerSortState.asc = true;
            teamSortState.col = -1;
            teamSortState.asc = true;
        }
        selectedServer = address;
        switchTab("servers");
        renderServers();
        renderServerDetail(true);
    };

    // ===== Render Server Detail =====
    function renderServerDetail(userSelected) {
        var emptyEl = document.getElementById("serverDetailEmpty");
        var contentEl = document.getElementById("serverDetailContent");

        if (!selectedServer || !serversData || !serversData.servers) {
            emptyEl.style.display = "";
            contentEl.style.display = "none";
            return;
        }

        var s = null;
        for (var i = 0; i < serversData.servers.length; i++) {
            if (serversData.servers[i].address === selectedServer) {
                s = serversData.servers[i];
                break;
            }
        }
        if (!s) {
            emptyEl.textContent = "Server no longer available";
            emptyEl.style.display = "";
            contentEl.style.display = "none";
            return;
        }

        var np = s["num-players"] || 0;
        var mp = s["max-players"] || 0;

        var html = '<div class="detail-inner">';

        // Title bar
        html += '<div class="detail-title-bar">';
        html += '<div>';
        html += '<span class="detail-title">' + esc(s.name) + '</span>';
        html += '<span class="detail-subtitle">' + esc(s.address) + '</span>';
        html += '</div>';
        html += '<button class="btn btn-close" onclick="selectServer(null)">Close</button>';
        html += '</div>';

        // Body: info column + lists column
        html += '<div class="detail-body">';

        // -- Info column --
        html += '<div class="detail-info-col">';

        html += '<div class="info-group">';
        html += '<div class="info-group-title">Server</div>';
        html += infoRow("Map", s.mission);
        html += infoRow("Type", s["server-type"]);
        html += infoRow("Mod", s.mod);
        html += infoRow("Game", s.game);
        html += infoRow("Version", s.version);
        html += '</div>';

        html += '<div class="info-group">';
        html += '<div class="info-group-title">Status</div>';
        var plClass = np === 0 ? "" : (np >= mp ? " warn" : " bright");
        html += infoRow("Players", np + " / " + mp, plClass);
        html += infoRow("Ping", (s.ping || 0) + " ms");
        html += infoRow("Dedicated", s.dedicated ? "Yes" : "No");
        html += infoRow("Password", s.password ? "Yes" : "No", s.password ? " warn" : "");
        html += infoRow("CPU", (s["cpu-speed"] || 0) + " MHz");
        html += '</div>';

        html += '<div class="info-group">';
        html += '<div class="info-group-title">Tracking</div>';
        html += infoRow("First Seen", formatTime(s["first-seen"]));
        html += infoRow("Last Seen", formatTime(s["last-seen"]));
        html += '</div>';

        if (s.info) {
            html += '<div class="info-group">';
            html += '<div class="info-group-title">Info</div>';
            html += '<div class="server-info-text">' + formatTribesInfo(s.info) + '</div>';
            html += '</div>';
        }

        html += '</div>'; // detail-info-col

        // -- Lists column --
        html += '<div class="detail-lists-col">';

        // Teams
        var teamCols = parseScoreHeader(s["team-score-header"]);
        var hasTeams = s.teams && s.teams.length > 0 && teamCols.length > 0;
        if (hasTeams) {
            // Build expanded rows for sorting
            var teamRows = [];
            for (var t = 0; t < s.teams.length; t++) {
                teamRows.push(expandScore(s.teams[t].score, {
                    "%t": s.teams[t].name
                }));
            }
            // Sort if a column is selected
            if (teamSortState.col >= 0 && teamSortState.col < teamCols.length) {
                var tsc = teamSortState.col;
                var tsa = teamSortState.asc;
                teamRows.sort(function (a, b) {
                    var va = (a[tsc] || ""), vb = (b[tsc] || "");
                    var na = parseFloat(va), nb = parseFloat(vb);
                    if (!isNaN(na) && !isNaN(nb)) return tsa ? na - nb : nb - na;
                    va = va.toLowerCase(); vb = vb.toLowerCase();
                    if (va < vb) return tsa ? -1 : 1;
                    if (va > vb) return tsa ? 1 : -1;
                    return 0;
                });
            }
            html += '<div class="sub-list sub-list-teams">';
            html += '<div class="sub-list-header">Teams</div>';
            html += '<div class="sub-list-scroll"><table>';
            html += '<thead><tr>';
            for (var tc = 0; tc < teamCols.length; tc++) {
                var tcAlign = tc === 0 ? '' : ' text-right';
                var tcSorted = '';
                if (teamSortState.col === tc) tcSorted = teamSortState.asc ? ' sorted-asc' : ' sorted-desc';
                html += '<th class="' + tcAlign + tcSorted + '" data-team-sort="' + tc + '">' + esc(teamCols[tc]) + '</th>';
            }
            html += '</tr></thead><tbody>';
            for (var tr2 = 0; tr2 < teamRows.length; tr2++) {
                html += '<tr>';
                for (var tv = 0; tv < teamCols.length; tv++) {
                    var tvAlign = tv === 0 ? '' : ' class="text-right"';
                    html += '<td' + tvAlign + '>' + esc(teamRows[tr2][tv] || "") + '</td>';
                }
                html += '</tr>';
            }
            html += '</tbody></table></div></div>';
        }

        // Players
        var hasPlayers = s.players && s.players.length > 0;
        if (hasPlayers) {
            var playerCols = parseScoreHeader(s["player-score-header"]);
            // Build expanded rows for sorting
            var playerRows = [];
            for (var p = 0; p < s.players.length; p++) {
                var pl = s.players[p];
                var plTeamName = "";
                if (pl.team === 255) plTeamName = "Observer";
                else if (s.teams && s.teams[pl.team]) plTeamName = s.teams[pl.team].name;
                playerRows.push(expandScore(pl.score, {
                    "%n": pl.name,
                    "%t": plTeamName || String(pl.team),
                    "%p": String(pl.ping * 4),
                    "%l": String(pl.pl)
                }));
            }
            // Sort if a column is selected
            if (playerSortState.col >= 0 && playerSortState.col < playerCols.length) {
                var sc = playerSortState.col;
                var sa = playerSortState.asc;
                playerRows.sort(function (a, b) {
                    var va = (a[sc] || ""), vb = (b[sc] || "");
                    var na = parseFloat(va), nb = parseFloat(vb);
                    if (!isNaN(na) && !isNaN(nb)) return sa ? na - nb : nb - na;
                    va = va.toLowerCase(); vb = vb.toLowerCase();
                    if (va < vb) return sa ? -1 : 1;
                    if (va > vb) return sa ? 1 : -1;
                    return 0;
                });
            }
            html += '<div class="sub-list sub-list-players">';
            html += '<div class="sub-list-header">Players</div>';
            html += '<div class="sub-list-scroll"><table>';
            html += '<thead><tr>';
            for (var pc = 0; pc < playerCols.length; pc++) {
                var pcAlign = pc === 0 ? '' : ' text-right';
                var pcSorted = '';
                if (playerSortState.col === pc) pcSorted = playerSortState.asc ? ' sorted-asc' : ' sorted-desc';
                html += '<th class="' + pcAlign + pcSorted + '" data-player-sort="' + pc + '">' + esc(playerCols[pc]) + '</th>';
            }
            html += '</tr></thead><tbody>';
            for (var pr = 0; pr < playerRows.length; pr++) {
                html += '<tr>';
                for (var pv = 0; pv < playerCols.length; pv++) {
                    var pvAlign = pv === 0 ? '' : ' class="text-right"';
                    html += '<td' + pvAlign + '>' + esc(playerRows[pr][pv] || "") + '</td>';
                }
                html += '</tr>';
            }
            html += '</tbody></table></div></div>';
        }

        if (!hasTeams && !hasPlayers) {
            html += '<div style="color:var(--text-dim);font-size:11px;padding:10px;text-transform:uppercase;letter-spacing:1px">No players connected</div>';
        }

        html += '</div>'; // detail-lists-col
        html += '</div>'; // detail-body
        html += '</div>'; // detail-inner

        // Save scroll positions before replacing content
        var scrolls = [];
        var scrollEls = contentEl.querySelectorAll(".sub-list-scroll");
        for (var si = 0; si < scrollEls.length; si++) {
            scrolls.push(scrollEls[si].scrollTop);
        }

        emptyEl.style.display = "none";
        contentEl.innerHTML = html;
        contentEl.style.display = "";

        // Restore scroll positions
        var newScrollEls = contentEl.querySelectorAll(".sub-list-scroll");
        for (var ri = 0; ri < newScrollEls.length && ri < scrolls.length; ri++) {
            newScrollEls[ri].scrollTop = scrolls[ri];
        }

        // Attach team sort handlers
        contentEl.querySelectorAll("th[data-team-sort]").forEach(function (th) {
            th.addEventListener("click", function () {
                var col = parseInt(th.getAttribute("data-team-sort"), 10);
                if (teamSortState.col === col) {
                    teamSortState.asc = !teamSortState.asc;
                } else {
                    teamSortState.col = col;
                    teamSortState.asc = true;
                }
                renderServerDetail();
            });
        });

        // Attach player sort handlers
        contentEl.querySelectorAll("th[data-player-sort]").forEach(function (th) {
            th.addEventListener("click", function () {
                var col = parseInt(th.getAttribute("data-player-sort"), 10);
                if (playerSortState.col === col) {
                    playerSortState.asc = !playerSortState.asc;
                } else {
                    playerSortState.col = col;
                    playerSortState.asc = true;
                }
                renderServerDetail();
            });
        });

        if (userSelected) {
            var pane = document.getElementById("serverDetailPane");
            if (pane) pane.scrollIntoView({ behavior: "smooth", block: "nearest" });
        }
    }

    function infoRow(key, val, extraClass) {
        return '<div class="info-row"><span class="info-key">' + esc(key) +
            '</span><span class="info-val' + (extraClass || '') + '">' +
            esc(String(val || "-")) + '</span></div>';
    }

    // ===== Render Masters List =====
    function renderMasters() {
        if (!mastersData || !mastersData.masters) return;

        var masters = mastersData.masters;
        if (!masters.length) {
            showEmpty("mastersLoading", "mastersContent", "No master servers configured");
            return;
        }

        var sorted = sortArray(masters, sortState.masters.key, sortState.masters.asc);
        updateSortHeaders("masters");

        var html = "";
        for (var i = 0; i < sorted.length; i++) {
            var m = sorted[i];
            var sel = (m.address === selectedMaster) ? " row-selected" : "";
            var motd = stripTribes(m["message-of-the-day"]);

            html += '<tr class="clickable' + sel + '" onclick="selectMaster(\'' + escAttr(m.address) + '\')">';
            var addrHtml = esc(m.address);
            if (m.local) addrHtml += '<span class="local-badge">LOCAL</span>';
            html += '<td>' + addrHtml + '</td>';
            html += '<td class="text-right">' + (m["server-count"] || 0) + '</td>';
            html += '<td class="text-right">' + (m.local ? "-" : (m.ping || 0)) + '</td>';
            html += '<td>' + (m["last-reply"] ? formatTime(m["last-reply"]) : "-") + '</td>';
            html += '<td>' + esc(motd || "-") + '</td>';
            html += '</tr>';
        }

        document.getElementById("mastersBody").innerHTML = html;
        document.getElementById("mastersLoading").style.display = "none";
        document.getElementById("mastersContent").style.display = "";
    }

    window.selectMaster = function (address) {
        selectedMaster = (selectedMaster === address) ? null : address;
        renderMasters();
        renderMasterDetail();
    };

    // ===== Render Master Detail =====
    function renderMasterDetail() {
        var emptyEl = document.getElementById("masterDetailEmpty");
        var contentEl = document.getElementById("masterDetailContent");

        if (!selectedMaster || !mastersData || !mastersData.masters) {
            emptyEl.style.display = "";
            contentEl.style.display = "none";
            return;
        }

        var m = null;
        for (var i = 0; i < mastersData.masters.length; i++) {
            if (mastersData.masters[i].address === selectedMaster) {
                m = mastersData.masters[i];
                break;
            }
        }
        if (!m) {
            emptyEl.textContent = "Master no longer available";
            emptyEl.style.display = "";
            contentEl.style.display = "none";
            return;
        }

        var html = '<div class="detail-inner">';

        // Title bar
        html += '<div class="detail-title-bar">';
        html += '<div>';
        html += '<span class="detail-title">' + esc(m.address) + '</span>';
        if (m.local) html += '<span class="local-badge">LOCAL</span>';
        html += '</div>';
        html += '<button class="btn btn-close" onclick="selectMaster(null)">Close</button>';
        html += '</div>';

        // Body
        html += '<div class="detail-body">';

        // Info column
        html += '<div class="detail-info-col">';
        html += '<div class="info-group">';
        html += '<div class="info-group-title">Master Server</div>';
        html += infoRow("Address", m.address);
        html += infoRow("Servers", m["server-count"] || 0);
        if (!m.local) {
            html += infoRow("Ping", (m.ping || 0) + " ms");
            html += infoRow("Last Reply", formatTime(m["last-reply"]));
        }
        html += '</div>';

        var motdRaw = m["message-of-the-day"] || "";
        if (motdRaw) {
            html += '<div class="info-group">';
            html += '<div class="info-group-title">MOTD</div>';
            html += '<div class="server-info-text">' + formatTribesInfo(motdRaw) + '</div>';
            html += '</div>';
        }
        html += '</div>'; // detail-info-col

        // Server list column
        html += '<div class="detail-lists-col">';
        var servers = m.servers;
        if (servers && servers.length > 0) {
            html += '<div class="sub-list" style="max-height:240px">';
            html += '<div class="sub-list-header">Server Addresses (' + servers.length + ')</div>';
            html += '<div class="sub-list-scroll"><table><tbody>';
            for (var s = 0; s < servers.length; s++) {
                html += '<tr class="clickable" onclick="goToServer(\'' + escAttr(servers[s]) + '\')"><td style="font-size:11px">' + esc(servers[s]) + '</td></tr>';
            }
            html += '</tbody></table></div></div>';
        } else {
            html += '<div style="color:var(--text-dim);font-size:11px;padding:10px;text-transform:uppercase;letter-spacing:1px">No servers reported</div>';
        }
        html += '</div>'; // detail-lists-col

        html += '</div>'; // detail-body
        html += '</div>'; // detail-inner

        emptyEl.style.display = "none";
        contentEl.innerHTML = html;
        contentEl.style.display = "";
    }

    // ===== Setup Tab =====
    var setupLines = "";

    function renderSetup() {
        if (!mastersData || !mastersData.masters) return;

        // Local first, then remaining sorted alphabetically by name
        var local = [];
        var remote = [];
        for (var j = 0; j < mastersData.masters.length; j++) {
            var m = mastersData.masters[j];
            if (m.local) local.push(m);
            else remote.push(m);
        }
        remote.sort(function (a, b) {
            var na = (a.name || "").toLowerCase();
            var nb = (b.name || "").toLowerCase();
            return na < nb ? -1 : na > nb ? 1 : 0;
        });
        var masters = local.concat(remote);

        var addrs = [];
        for (var i = 0; i < masters.length; i++) {
            var addr = masters[i].address || "";
            if (addr) addrs.push(addr);
        }
        var line = '$Server::MasterAddressN0 = "' + addrs.join(" ") + '";';
        var configLines = [line];
        var consoleLines = [line];

        setupLines = configLines.join("\n");

        var masterEl = document.getElementById("setupMasterLines");
        if (masterEl) masterEl.textContent = setupLines;

        var consoleEl = document.getElementById("setupConsoleLines");
        if (consoleEl) consoleEl.textContent = consoleLines.join("\n");
    }

    window.copySetupLines = function () {
        if (!setupLines) return;
        var btn = document.getElementById("setupCopyBtn");
        if (navigator.clipboard) {
            navigator.clipboard.writeText(setupLines).then(function () {
                if (btn) { btn.textContent = "Copied!"; setTimeout(function () { btn.textContent = "Copy to Clipboard"; }, 2000); }
            });
        } else {
            // Fallback
            var ta = document.createElement("textarea");
            ta.value = setupLines;
            ta.style.position = "fixed";
            ta.style.opacity = "0";
            document.body.appendChild(ta);
            ta.select();
            document.execCommand("copy");
            document.body.removeChild(ta);
            if (btn) { btn.textContent = "Copied!"; setTimeout(function () { btn.textContent = "Copy to Clipboard"; }, 2000); }
        }
    };

    // ===== Helpers =====
    function showEmpty(loadingId, contentId, msg) {
        var el = document.getElementById(loadingId);
        el.textContent = msg;
        el.className = "status-msg";
        el.style.display = "";
        document.getElementById(contentId).style.display = "none";
    }

    function esc(str) {
        if (!str) return "";
        var div = document.createElement("div");
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    function escAttr(str) {
        return esc(str).replace(/'/g, "&#39;").replace(/"/g, "&quot;");
    }

    function formatTribesInfo(str) {
        if (!str) return "";
        var fontSizes = { f0: "16px", f1: "13px", f2: "11px", f3: "10px" };
        var result = "";
        var align = "left";
        var size = "11px";
        // Split on actual newlines and literal \n sequences
        var lines = str.replace(/\\n/g, "\n").split("\n");
        for (var li = 0; li < lines.length; li++) {
            if (li > 0) result += "<br>";
            var line = lines[li];
            var pos = 0;
            var lineAlign = align;
            var lineSize = size;
            var text = "";
            while (pos < line.length) {
                var tagMatch = line.substring(pos).match(/^<(jc|jl|jr|f[0-3]|n)>/i);
                if (tagMatch) {
                    var tag = tagMatch[1].toLowerCase();
                    if (tag === "n") {
                        // Flush current text as a div, then start a new line
                        result += '<div style="text-align:' + lineAlign + ';font-size:' + lineSize + '">' + esc(text) + '</div>';
                        text = "";
                    } else if (tag === "jc") lineAlign = "center";
                    else if (tag === "jl") lineAlign = "left";
                    else if (tag === "jr") lineAlign = "right";
                    else if (fontSizes[tag]) lineSize = fontSizes[tag];
                    pos += tagMatch[0].length;
                } else {
                    text += line[pos];
                    pos++;
                }
            }
            // Carry alignment and size state forward
            align = lineAlign;
            size = lineSize;
            result += '<div style="text-align:' + lineAlign + ';font-size:' + lineSize + '">' + esc(text) + '</div>';
        }
        return result;
    }

    function stripTribes(str) {
        if (!str) return "";
        return str.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
    }

    function parseScoreHeader(header) {
        if (!header) return [];
        return header.split("\t").map(function (col) {
            return col.replace(/[^\x20-\x7E]/g, "").trim();
        }).filter(function (col) { return col.length > 0; });
    }

    function expandScore(score, vars) {
        if (!score) return [];
        return score.split("\t").map(function (val) {
            val = val.trim();
            if (vars[val] !== undefined) return vars[val];
            return val;
        });
    }

    function formatTime(str) {
        if (!str) return "-";
        try {
            var d = new Date(str);
            if (isNaN(d.getTime())) return str;
            return d.toLocaleString();
        } catch (e) {
            return str;
        }
    }

    // ===== Matrix Easter Egg =====
    var matrixSeq = "matrix";
    var matrixBuf = "";
    var matrixAnim = null;

    document.addEventListener("keypress", function (e) {
        if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA") return;
        matrixBuf += e.key.toLowerCase();
        if (matrixBuf.length > matrixSeq.length) matrixBuf = matrixBuf.slice(-matrixSeq.length);
        if (matrixBuf === matrixSeq) {
            matrixBuf = "";
            toggleMatrix();
        }
    });

    function toggleMatrix() {
        var active = document.body.classList.toggle("matrix-active");
        if (active) {
            startMatrix();
        } else {
            stopMatrix();
        }
    }

    function startMatrix() {
        var canvas = document.getElementById("matrixCanvas");
        var ctx = canvas.getContext("2d");
        var fontSize = 14;
        var columns;
        var drops;

        function resize() {
            canvas.width = window.innerWidth;
            canvas.height = window.innerHeight;
            columns = Math.floor(canvas.width / fontSize);
            var oldDrops = drops || [];
            drops = [];
            for (var i = 0; i < columns; i++) {
                drops[i] = (i < oldDrops.length) ? oldDrops[i] : Math.random() * -100;
            }
        }

        resize();
        window.addEventListener("resize", resize);
        canvas._resizeHandler = resize;

        var chars = "abcdefghijklmnopqrstuvwxyz0123456789@#$%^&*(){}[]|;:<>,.?/~`";

        function draw() {
            ctx.fillStyle = "rgba(0, 0, 0, 0.05)";
            ctx.fillRect(0, 0, canvas.width, canvas.height);
            ctx.font = fontSize + "px monospace";

            for (var i = 0; i < columns; i++) {
                var ch = chars[Math.floor(Math.random() * chars.length)];
                var y = drops[i] * fontSize;

                // Bright leading character
                ctx.fillStyle = "#aaffaa";
                ctx.fillText(ch, i * fontSize, y);

                // Trail characters are dimmer green
                ctx.fillStyle = "#00bb00";
                if (Math.random() > 0.95) ctx.fillStyle = "#00ff00";

                drops[i]++;
                if (y > canvas.height && Math.random() > 0.975) {
                    drops[i] = 0;
                }
            }
            matrixAnim = requestAnimationFrame(draw);
        }

        draw();
    }

    function stopMatrix() {
        if (matrixAnim) {
            cancelAnimationFrame(matrixAnim);
            matrixAnim = null;
        }
        var canvas = document.getElementById("matrixCanvas");
        if (canvas._resizeHandler) {
            window.removeEventListener("resize", canvas._resizeHandler);
            canvas._resizeHandler = null;
        }
        var ctx = canvas.getContext("2d");
        ctx.clearRect(0, 0, canvas.width, canvas.height);
    }

    // ===== Start =====
    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
