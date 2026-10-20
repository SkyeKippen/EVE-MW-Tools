// Shopping List page: fills the missing-items table from the ticked
// stockpiles and the pasted-list table from the Multibuy box, and copies
// either table out in Multibuy format.
(function () {
    const checksKey = "shopping.checkedStockpiles";
    const checks = Array.from(document.querySelectorAll(".stockpile-check"));
    const missingBody = document.querySelector("#missing-table tbody");
    const pastedBody = document.querySelector("#pasted-table tbody");
    const box = document.getElementById("multibuy");
    const rowsByTable = { "missing-table": [], "pasted-table": [] };

    function message(body, text) {
        body.replaceChildren();
        const tr = document.createElement("tr");
        const td = document.createElement("td");
        td.colSpan = 3;
        td.className = "empty";
        td.textContent = text;
        tr.appendChild(td);
        body.appendChild(tr);
    }

    function fill(tableId, body, rows, emptyText) {
        rowsByTable[tableId] = rows;
        if (rows.length === 0) {
            message(body, emptyText);
            return;
        }
        body.replaceChildren();
        rows.forEach(function (row) {
            const tr = document.createElement("tr");
            const icon = document.createElement("td");
            icon.className = "icon-col";
            const name = document.createElement("td");
            const quantity = document.createElement("td");
            quantity.className = "num";
            if (row.bad) {
                tr.className = "bad-row";
                tr.title = row.text;
                name.textContent = "BAD ITEM!";
                quantity.textContent = "BAD ITEM!";
            } else {
                const img = document.createElement("img");
                img.src = "/image?category=types&id=" + row.typeId +
                    "&variation=" + (row.blueprint ? "bp" : "icon") + "&size=32";
                img.alt = "";
                img.width = 32;
                img.height = 32;
                img.loading = "lazy";
                icon.appendChild(img);
                name.textContent = row.name;
                quantity.textContent = row.quantity.toLocaleString("en-US", { maximumFractionDigits: 0 });
            }
            tr.append(icon, name, quantity);
            body.appendChild(tr);
        });
    }

    // Missing items from the ticked stockpiles. A newer request replaces an
    // older one still on its way.
    let missingRequest = 0;
    function loadMissing() {
        const ids = checks.filter(function (c) { return c.checked; }).map(function (c) { return c.value; });
        try {
            localStorage.setItem(checksKey, JSON.stringify(ids));
        } catch (e) { /* storage off; the ticks just aren't remembered */ }
        const request = ++missingRequest;
        if (ids.length === 0) {
            fill("missing-table", missingBody, [], "Tick a stockpile to see what it's missing.");
            return;
        }
        message(missingBody, "Loading…");
        fetch("/shopping/missing?ids=" + encodeURIComponent(ids.join(",")))
            .then(function (res) {
                if (!res.ok) throw new Error(res.status);
                return res.json();
            })
            .then(function (rows) {
                if (request !== missingRequest) return;
                fill("missing-table", missingBody, rows, "The ticked stockpiles have everything they want.");
            })
            .catch(function () {
                if (request !== missingRequest) return;
                rowsByTable["missing-table"] = [];
                message(missingBody, "Couldn't load what's missing. Reload the page to try again.");
            });
    }

    // The pasted list, checked a moment after typing stops.
    let parseRequest = 0;
    let parseTimer = 0;
    function parseList() {
        const request = ++parseRequest;
        if (box.value.trim() === "") {
            fill("pasted-table", pastedBody, [], "Paste a list above to check it.");
            return;
        }
        fetch("/shopping/parse", {
            method: "POST",
            body: new URLSearchParams({ list: box.value })
        })
            .then(function (res) {
                if (!res.ok) throw new Error(res.status);
                return res.json();
            })
            .then(function (rows) {
                if (request !== parseRequest) return;
                fill("pasted-table", pastedBody, rows, "Paste a list above to check it.");
            })
            .catch(function () {
                if (request !== parseRequest) return;
                rowsByTable["pasted-table"] = [];
                message(pastedBody, "Couldn't check the list. Reload the page to try again.");
            });
    }

    function copyTable(button) {
        const lines = rowsByTable[button.dataset.copy]
            .filter(function (row) { return !row.bad; })
            .map(function (row) { return row.name + " " + row.quantity; });
        const label = button.textContent;
        function show(text) {
            button.textContent = text;
            setTimeout(function () { button.textContent = label; }, 1500);
        }
        if (lines.length === 0) {
            show("Nothing to copy");
            return;
        }
        navigator.clipboard.writeText(lines.join("\n")).then(
            function () { show("Copied!"); },
            function () { show("Couldn't copy"); });
    }

    let saved = [];
    try {
        saved = JSON.parse(localStorage.getItem(checksKey) || "[]");
    } catch (e) { /* storage off or unreadable */ }
    checks.forEach(function (check) {
        check.checked = saved.indexOf(check.value) >= 0;
        check.addEventListener("change", loadMissing);
    });
    document.querySelectorAll("[data-copy]").forEach(function (button) {
        button.addEventListener("click", function () { copyTable(button); });
    });
    box.addEventListener("input", function () {
        clearTimeout(parseTimer);
        parseTimer = setTimeout(parseList, 300);
    });

    loadMissing();
    parseList();
})();
