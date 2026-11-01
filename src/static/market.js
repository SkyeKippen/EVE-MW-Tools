// Import page: copies the Multibuy lists. Prices page: draws the chart of
// one item's saved daily prices and CCP's market history.
(function () {
    document.querySelectorAll("[data-copy-text]").forEach(function (button) {
        button.addEventListener("click", function () {
            const box = document.getElementById(button.dataset.copyText);
            const label = button.textContent;
            navigator.clipboard.writeText(box.value).then(function () {
                button.textContent = "Copied";
            }, function () {
                box.select();
                button.textContent = "Press Ctrl+C to copy";
            });
            setTimeout(function () { button.textContent = label; }, 2000);
        });
    });

    const dataEl = document.getElementById("price-data");
    const canvas = document.getElementById("price-chart");
    if (!dataEl || !canvas || !window.Chart) return;
    const data = JSON.parse(dataEl.textContent || "{}") || {};
    const history = data.history || [];
    const points = data.points || [];
    if (history.length === 0 && points.length === 0) {
        canvas.parentElement.textContent = "No history or saved prices for this item yet.";
        return;
    }

    function shortIsk(v) {
        const abs = Math.abs(v);
        const units = [[1e12, "t"], [1e9, "b"], [1e6, "m"], [1e3, "k"]];
        for (const [size, suffix] of units) {
            if (abs >= size) return (v / size).toFixed(2) + suffix;
        }
        return v.toFixed(abs < 10 ? 2 : 0);
    }
    const fullIsk = (v) => v.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2}) + " ISK";

    const css = getComputedStyle(document.documentElement);
    const token = (name) => css.getPropertyValue(name).trim();
    const day = (s) => Date.parse(s + (s.length === 10 ? "T00:00:00Z" : ""));

    function line(label, data, color, dashed) {
        return {
            type: "line", label, data, yAxisID: "y",
            borderColor: color, backgroundColor: color,
            borderWidth: 2, borderDash: dashed ? [6, 4] : [],
            pointRadius: data.length > 60 ? 0 : 2, pointHoverRadius: 4,
            spanGaps: true, tension: 0,
        };
    }

    const colors = {jita: token("--accent"), cj6mt: token("--good"), plex: token("--plex")};
    const datasets = [];
    if (history.length > 0) {
        datasets.push(line(data.region + " average", history.map((h) => ({x: day(h.date), y: h.average})), token("--pending"), false));
    }
    for (const market of Object.keys(colors)) {
        const mine = points.filter((p) => p.market === market);
        if (mine.length === 0) continue;
        const name = (data.names || {})[market] || market;
        const sells = mine.filter((p) => p.sell != null).map((p) => ({x: day(p.date), y: p.sell}));
        const buys = mine.filter((p) => p.buy != null).map((p) => ({x: day(p.date), y: p.buy}));
        if (sells.length > 0) datasets.push(line(name + " sell", sells, colors[market], false));
        if (buys.length > 0) datasets.push(line(name + " buy", buys, colors[market], true));
    }
    if (history.length > 0) {
        datasets.push({
            type: "bar", label: "Volume traded", yAxisID: "volume",
            data: history.map((h) => ({x: day(h.date), y: h.volume})),
            backgroundColor: color(token("--muted"), 0.35), barPercentage: 1, categoryPercentage: 1,
        });
    }

    // "#6b7280", 0.35 -> "rgba(107, 114, 128, 0.35)"; canvases can't take color-mix()
    function color(hex, alpha) {
        const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
        if (!m) return hex;
        return "rgba(" + parseInt(m[1], 16) + ", " + parseInt(m[2], 16) + ", " + parseInt(m[3], 16) + ", " + alpha + ")";
    }

    const times = datasets.flatMap((d) => d.data.map((p) => p.x));
    const dateFormat = new Intl.DateTimeFormat(undefined, {day: "numeric", month: "short", year: "2-digit", timeZone: "UTC"});

    new Chart(canvas, {
        data: {datasets},
        options: {
            maintainAspectRatio: false,
            animation: false,
            interaction: {mode: "nearest", axis: "x", intersect: false},
            color: token("--muted"),
            scales: {
                x: {
                    type: "linear", min: Math.min(...times), max: Math.max(...times),
                    grid: {display: false},
                    ticks: {color: token("--muted"), maxTicksLimit: 8, callback: (v) => dateFormat.format(v)},
                },
                y: {
                    position: "left",
                    grid: {color: token("--line")},
                    ticks: {color: token("--muted"), callback: (v) => shortIsk(v)},
                },
                volume: {
                    position: "right", display: history.length > 0, beginAtZero: true,
                    grid: {display: false},
                    ticks: {color: token("--muted"), callback: (v) => shortIsk(v)},
                },
            },
            plugins: {
                legend: {position: "bottom", labels: {color: token("--text"), boxWidth: 12, boxHeight: 12}},
                tooltip: {
                    callbacks: {
                        title: (items) => dateFormat.format(items[0].parsed.x),
                        label: (item) => item.dataset.label + ": " +
                            (item.dataset.yAxisID === "volume" ? item.parsed.y.toLocaleString() + " units" : fullIsk(item.parsed.y)),
                    },
                },
            },
        },
    });
})();
