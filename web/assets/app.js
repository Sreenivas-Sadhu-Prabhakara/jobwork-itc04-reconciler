/* Job-work challan register and ITC-04 reconciler — the page's own behaviour.
   Every figure comes from the tool running on this machine; nothing else is
   contacted. The page builds its tables as text nodes, never as markup, so a
   job worker's name can never be read as anything but a name. */
(function () {
  "use strict";

  var MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  var moneyFmt = new Intl.NumberFormat("en-IN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  var qtyFmt = new Intl.NumberFormat("en-IN", { maximumFractionDigits: 3 });

  var state = null;
  var filing = null;
  var frequency = "half-yearly";

  /* ---------- small helpers ---------- */

  function $(id) { return document.getElementById(id); }

  function setText(node, s) { if (node) { node.textContent = s; } }

  function show(node, on) {
    if (!node) { return; }
    if (on) { node.removeAttribute("hidden"); } else { node.setAttribute("hidden", ""); }
  }

  function clear(node) {
    while (node && node.firstChild) { node.removeChild(node.firstChild); }
  }

  function rupees(v) { return "₹" + moneyFmt.format(Number(v) || 0); }

  function qty(v) { return qtyFmt.format(Number(v) || 0); }

  function qtyUnit(v, unit) { return qty(v) + (unit ? " " + unit : ""); }

  function dateText(iso) {
    if (!iso) { return "—"; }
    var parts = String(iso).split("-");
    if (parts.length !== 3) { return String(iso); }
    var month = MONTHS[Number(parts[1]) - 1] || parts[1];
    return Number(parts[2]) + " " + month + " " + parts[0];
  }

  function plural(n, one, many) { return n + " " + (Math.abs(n) === 1 ? one : many); }

  function td(text, cls) {
    var cell = document.createElement("td");
    if (cls) { cell.className = cls; }
    cell.appendChild(document.createTextNode(text));
    return cell;
  }

  function tdLines(lines, cls) {
    var cell = document.createElement("td");
    if (cls) { cell.className = cls; }
    lines.forEach(function (line, i) {
      if (i > 0) { cell.appendChild(document.createElement("br")); }
      var span = document.createElement("span");
      if (line.cls) { span.className = line.cls; }
      span.appendChild(document.createTextNode(line.text));
      cell.appendChild(span);
    });
    return cell;
  }

  function statePill(name) {
    var span = document.createElement("span");
    span.className = name === "Overdue" ? "pill neg" : (name === "Pending" ? "pill reg" : "pill");
    span.appendChild(document.createTextNode(name));
    return span;
  }

  function smallButton(label, title, handler) {
    var button = document.createElement("button");
    button.type = "button";
    button.className = "themer";
    button.setAttribute("aria-label", title);
    button.appendChild(document.createTextNode(label));
    button.addEventListener("click", handler);
    return button;
  }

  function message(node, text, problems) {
    if (!node) { return; }
    clear(node);
    if (!text) { show(node, false); return; }
    node.appendChild(document.createTextNode(text));
    if (problems && problems.length > 1) {
      var list = document.createElement("ul");
      problems.forEach(function (p) {
        var item = document.createElement("li");
        item.appendChild(document.createTextNode(p));
        list.appendChild(item);
      });
      node.appendChild(list);
    }
    show(node, true);
  }

  function unitLabel(units) {
    if (!units || units.length === 0) { return ""; }
    if (units.length === 1) { return units[0]; }
    return "(mixed units)";
  }

  function banner(text) { message($("banner"), text || ""); }

  /* ---------- talking to the tool ---------- */

  function api(method, url, body, contentType) {
    var options = { method: method, headers: {} };
    if (body !== undefined && body !== null) {
      options.headers["Content-Type"] = contentType || "application/json";
      options.body = contentType ? body : JSON.stringify(body);
    }
    return fetch(url, options).then(function (response) {
      return response.text().then(function (text) {
        var data = null;
        if (text) {
          try { data = JSON.parse(text); } catch (e) { data = null; }
        }
        if (!response.ok) {
          var err = new Error((data && data.error) || ("the tool answered " + response.status));
          err.problems = (data && data.problems) || null;
          throw err;
        }
        return data;
      });
    });
  }

  function load() {
    var params = new URLSearchParams();
    var asOf = $("as-of").value;
    var horizon = $("horizon").value;
    if (asOf) { params.set("asOf", asOf); }
    if (horizon !== "") { params.set("horizon", horizon); }
    return api("GET", "api/state?" + params.toString()).then(function (data) {
      state = data;
      banner("");
      render();
    }).catch(function (err) {
      banner("Could not read the register: " + err.message);
    });
  }

  /* ---------- drawing the page ---------- */

  function render() {
    if (!state) { return; }
    if (!$("as-of").value) { $("as-of").value = state.asOf || state.today || ""; }
    if ($("horizon").value === "") { $("horizon").value = state.dashboard.horizonDays; }
    renderDashboard();
    renderRegister();
    renderRecordForm();
    renderSettings();
    renderPeriods();
    setText($("s-datafile"), state.dataFile || "—");
  }

  function renderDashboard() {
    var dash = state.dashboard;
    var totals = dash.totals;

    setText($("d-overdue-value"), rupees(totals.valueOverdue));
    setText($("d-overdue-sub"), totals.overdue === 0
      ? "Nothing is past its return deadline as at " + dateText(state.asOf)
      : plural(totals.overdue, "challan", "challans") + " past the return deadline as at " + dateText(state.asOf));

    setText($("d-challans"), String(totals.challans));
    setText($("d-returned"), String(totals.returned));
    setText($("d-pending"), String(totals.pending));
    setText($("d-overdue"), String(totals.overdue));
    setText($("d-qty-sent"), qty(totals.qtySent));
    setText($("d-qty-returned"), qty(totals.qtyReturned));
    setText($("d-qty-out"), qty(totals.qtyOutstanding));
    setText($("d-value-out"), rupees(totals.valueOutstanding));
    setText($("d-workers"), String(totals.jobWorkers));
    show($("d-units"), !!totals.mixedUnits);

    var atRisk = dash.atRisk || [];
    var body = $("d-atrisk");
    clear(body);
    atRisk.forEach(function (a) {
      var row = document.createElement("tr");
      row.appendChild(tdLines([{ text: a.challanNo }, { text: dateText(a.sentDate) }]));
      row.appendChild(td(a.jobWorkerName || a.jobWorkerGstin));
      row.appendChild(td(qtyUnit(a.outstandingQty, a.uqc), "num"));
      row.appendChild(tdLines([
        { text: dateText(a.deadline) },
        a.state === "Overdue"
          ? { text: plural(a.daysOverdue, "day", "days") + " over", cls: "neg" }
          : { text: "in " + plural(a.daysLeft, "day", "days") }
      ]));
      row.appendChild(td(rupees(a.deemedSupplyValue), "num"));
      body.appendChild(row);
    });
    show($("d-atrisk-table"), atRisk.length > 0);
    show($("d-atrisk-empty"), atRisk.length === 0);

    var workers = dash.workers || [];
    var workerBody = $("d-workers-body");
    clear(workerBody);
    workers.forEach(function (w) {
      var row = document.createElement("tr");
      row.appendChild(tdLines([{ text: w.jobWorkerName || "—" }, { text: w.jobWorkerGstin }]));
      row.appendChild(tdLines([
        { text: String(w.challans) },
        w.overdueChallans > 0 ? { text: w.overdueChallans + " over", cls: "neg" } : { text: w.openChallans + " open" }
      ], "num"));
      row.appendChild(td(qtyUnit(w.qtyOutstanding, unitLabel(w.uqcs)), "num"));
      row.appendChild(td(w.openChallans === 0 ? "—" : dateText(w.earliestDeadline)));
      row.appendChild(td(rupees(w.valueOutstanding), "num"));
      workerBody.appendChild(row);
    });
    show($("d-workers-table"), workers.length > 0);
    show($("d-workers-empty"), workers.length === 0);

    var orphans = dash.orphanReturns || [];
    var orphanBody = $("d-orphans");
    clear(orphanBody);
    orphans.forEach(function (c) {
      var row = document.createElement("tr");
      row.appendChild(td(c.no));
      row.appendChild(td(dateText(c.date)));
      row.appendChild(td(c.againstChallanNo, "neg"));
      row.appendChild(td(qtyUnit(c.qty, c.uqc), "num"));
      orphanBody.appendChild(row);
    });
    show($("d-orphan-card"), orphans.length > 0);
  }

  function renderRegister() {
    var assessments = state.assessments || [];
    var body = $("r-sent");
    clear(body);
    assessments.forEach(function (a) {
      var row = document.createElement("tr");
      row.appendChild(tdLines([
        { text: a.challanNo },
        { text: dateText(a.sentDate) },
        { text: a.itemDesc + (a.hsn ? " · HSN " + a.hsn : "") }
      ]));
      row.appendChild(tdLines([
        { text: a.jobWorkerName || "—" },
        { text: a.goodsType === "capital" ? "Capital goods" : "Inputs" }
      ]));
      row.appendChild(td(qty(a.sentQty) + " / " + qty(a.returnedQty) + " / " + qty(a.outstandingQty) + " " + a.uqc, "num"));

      var status = document.createElement("td");
      status.appendChild(statePill(a.state));
      status.appendChild(document.createElement("br"));
      var detail = document.createElement("span");
      if (a.state === "Overdue") {
        detail.className = "neg";
        detail.appendChild(document.createTextNode(dateText(a.deadline) + " · " + plural(a.daysOverdue, "day", "days") + " over"));
      } else if (a.state === "Pending") {
        detail.appendChild(document.createTextNode(dateText(a.deadline) + " · in " + plural(a.daysLeft, "day", "days")));
      } else {
        detail.appendChild(document.createTextNode("settled in full"));
      }
      status.appendChild(detail);
      if (a.overReturned) {
        status.appendChild(document.createElement("br"));
        var excess = document.createElement("span");
        excess.className = "neg";
        excess.appendChild(document.createTextNode(qty(a.excessQty) + " more back than went out"));
        status.appendChild(excess);
      }
      row.appendChild(status);

      var actions = document.createElement("td");
      if (a.state !== "Returned") {
        actions.appendChild(smallButton("Return", "Record goods received back against " + a.challanNo, function () {
          openPanel("record");
          $("b-against").value = a.challanNo;
          onAgainstChange();
          $("b-qty").focus();
        }));
        actions.appendChild(document.createTextNode(" "));
      }
      actions.appendChild(smallButton("Remove", "Remove challan " + a.challanNo, function () {
        removeChallan(a.challanNo, a.returnCount);
      }));
      row.appendChild(actions);
      body.appendChild(row);
    });
    show($("r-sent-table"), assessments.length > 0);
    show($("r-sent-empty"), assessments.length === 0);

    var returns = (state.challans || []).filter(function (c) { return c.direction === "returned"; });
    var backBody = $("r-back");
    clear(backBody);
    returns.forEach(function (c) {
      var row = document.createElement("tr");
      row.appendChild(td(c.no));
      row.appendChild(td(dateText(c.date)));
      row.appendChild(td(c.againstChallanNo || "—"));
      row.appendChild(td(qtyUnit(c.qty, c.uqc), "num"));
      var actions = document.createElement("td");
      actions.appendChild(smallButton("Remove", "Remove return " + c.no, function () {
        removeChallan(c.no, 0);
      }));
      row.appendChild(actions);
      backBody.appendChild(row);
    });
    show($("r-back-table"), returns.length > 0);
    show($("r-back-empty"), returns.length === 0);
  }

  function renderRecordForm() {
    if (!$("s-date").value) { $("s-date").value = state.today || ""; }
    if (!$("b-date").value) { $("b-date").value = state.today || ""; }

    var open = (state.assessments || []).filter(function (a) { return a.state !== "Returned"; });
    var select = $("b-against");
    var previous = select.value;
    clear(select);

    if (open.length === 0) {
      var none = document.createElement("option");
      none.value = "";
      none.appendChild(document.createTextNode("No challan has anything still out"));
      select.appendChild(none);
    } else {
      var placeholder = document.createElement("option");
      placeholder.value = "";
      placeholder.appendChild(document.createTextNode("Choose a challan…"));
      select.appendChild(placeholder);
      open.forEach(function (a) {
        var option = document.createElement("option");
        option.value = a.challanNo;
        option.appendChild(document.createTextNode(
          a.challanNo + " · " + (a.jobWorkerName || a.jobWorkerGstin) +
          " · " + qtyUnit(a.outstandingQty, a.uqc) + " out"));
        select.appendChild(option);
      });
      if (previous) { select.value = previous; }
    }
    onAgainstChange();

    var seen = {};
    var list = $("gstin-list");
    clear(list);
    (state.challans || []).forEach(function (c) {
      if (!c.jobWorkerGstin || seen[c.jobWorkerGstin]) { return; }
      seen[c.jobWorkerGstin] = true;
      var option = document.createElement("option");
      option.value = c.jobWorkerGstin;
      option.setAttribute("label", c.jobWorkerName || c.jobWorkerGstin);
      list.appendChild(option);
    });
  }

  function findAssessment(challanNo) {
    var found = null;
    (state.assessments || []).forEach(function (a) {
      if (a.challanNo === challanNo) { found = a; }
    });
    return found;
  }

  function onAgainstChange() {
    var chosen = findAssessment($("b-against").value);
    if (!chosen) {
      $("b-uqc").value = "";
      setText($("b-position"), "Pick a challan to see how much of it is still out.");
      return;
    }
    $("b-uqc").value = chosen.uqc;
    setText($("b-position"), qtyUnit(chosen.outstandingQty, chosen.uqc) + " of " +
      qtyUnit(chosen.sentQty, chosen.uqc) + " still out · due " + dateText(chosen.deadline) +
      " · " + (chosen.state === "Overdue"
        ? plural(chosen.daysOverdue, "day", "days") + " past the deadline"
        : "in " + plural(chosen.daysLeft, "day", "days")));
  }

  function renderSettings() {
    var s = state.settings;
    $("p-name").value = s.principalName || "";
    $("p-gstin").value = s.principalGstin || "";
    $("p-turnover").value = s.annualAggregateTurnoverInr || "";
    $("p-horizon").value = s.horizonDays || "";
    $("p-inputs-years").value = state.statutory.inputsReturnYears;
    $("p-capital-years").value = state.statutory.capitalGoodsReturnYears;
    $("p-threshold").value = state.statutory.turnoverThresholdInr;
    $("p-dueday").value = state.statutory.filingDueDayOfMonth;
  }

  /* ---------- the filing period ---------- */

  function renderPeriods() {
    if (!$("f-asof").value) { $("f-asof").value = state.asOf || state.today || ""; }

    var buttons = $("f-freq").querySelectorAll("button");
    var i;
    for (i = 0; i < buttons.length; i++) {
      buttons[i].setAttribute("aria-pressed", buttons[i].getAttribute("data-freq") === frequency ? "true" : "false");
    }

    var select = $("f-period");
    var previous = select.value;
    clear(select);
    (state.periods || []).forEach(function (p) {
      if (p.frequency !== frequency) { return; }
      var option = document.createElement("option");
      option.value = p.fyStart + "|" + p.index;
      option.appendChild(document.createTextNode(p.label + " · due " + dateText(p.dueDate)));
      select.appendChild(option);
    });
    if (previous) { select.value = previous; }
    if (!select.value && select.options.length > 0) { select.value = select.options[0].value; }
    updateDownloadLinks();
  }

  function filingQuery() {
    var params = new URLSearchParams();
    var chosen = $("f-period").value.split("|");
    params.set("freq", frequency);
    if (chosen.length === 2) {
      params.set("fy", chosen[0]);
      params.set("index", chosen[1]);
    }
    if ($("f-asof").value) { params.set("asOf", $("f-asof").value); }
    return params.toString();
  }

  function updateDownloadLinks() {
    var query = filingQuery();
    $("f-csv").href = "api/itc04.csv?" + query;
    $("f-json").href = "api/itc04.json?" + query;
  }

  function prepareFiling() {
    updateDownloadLinks();
    return api("GET", "api/itc04?" + filingQuery()).then(function (data) {
      filing = data;
      renderFiling();
    }).catch(function (err) {
      show($("f-result"), false);
      banner("Could not prepare the period: " + err.message);
    });
  }

  function renderFiling() {
    if (!filing) { return; }
    var totals = filing.totals;

    setText($("f-title"), filing.period.label);
    setText($("f-value"), rupees(totals.taxableValueSent));
    setText($("f-sub"), "Form ITC-04 for this period is due " + dateText(filing.dueDate) +
      " · position judged as at " + dateText(filing.asOf));

    setText($("f-challans"), String(totals.challans));
    setText($("f-qty-sent"), qty(totals.qtySent));
    setText($("f-qty-back"), qty(filing.qtyReturnedInPeriod));
    setText($("f-qty-out"), qty(totals.qtyOutstanding));
    setText($("f-overdue"), String(totals.overdue));
    setText($("f-deemed"), rupees(totals.valueOverdue));
    show($("f-units"), !!totals.mixedUnits);

    var body = $("f-workers");
    clear(body);
    (filing.workers || []).forEach(function (w) {
      var row = document.createElement("tr");
      row.appendChild(tdLines([{ text: w.jobWorkerName || "—" }, { text: w.jobWorkerGstin }]));
      row.appendChild(td(qty(w.qtySent), "num"));
      row.appendChild(td(qty(w.qtyReturnedInPeriod), "num"));
      row.appendChild(td(qty(w.qtyOutstanding), "num"));
      row.appendChild(td(rupees(w.deemedSupplyValue), "num"));
      body.appendChild(row);
    });
    if ((filing.workers || []).length === 0) {
      var row = document.createElement("tr");
      row.appendChild(td("No challans in this period."));
      body.appendChild(row);
    }

    var note = "";
    if (filing.suggestedFrequency && filing.suggestedFrequency !== filing.period.frequency) {
      note = filing.annualAggregateTurnoverInr > 0
        ? "A turnover of " + rupees(filing.annualAggregateTurnoverInr) + " puts you on a " +
          filing.suggestedFrequency + " period under rule 45(3); this is a " + filing.period.frequency + " one."
        : "With no turnover stated the tool assumes the shorter, " + filing.suggestedFrequency +
          " period under rule 45(3); this is a " + filing.period.frequency + " one. Set your turnover under Settings.";
    }
    message($("f-freq-note"), note);
    show($("f-result"), true);
  }

  /* ---------- changing the register ---------- */

  function numberOrZero(id) {
    var raw = $(id).value;
    return raw === "" ? 0 : Number(raw);
  }

  function submitSent(event) {
    event.preventDefault();
    var body = {
      no: $("s-no").value,
      date: $("s-date").value,
      jobWorkerGstin: $("s-gstin").value,
      jobWorkerName: $("s-name").value,
      itemDesc: $("s-desc").value,
      hsn: $("s-hsn").value,
      qty: numberOrZero("s-qty"),
      uqc: $("s-uqc").value,
      taxableValue: numberOrZero("s-value"),
      goodsType: $("s-goods").value
    };
    api("POST", "api/challans", body).then(function (data) {
      message($("form-sent-msg"), "Challan " + data.challan.no + " recorded.");
      $("s-no").value = "";
      $("s-desc").value = "";
      $("s-qty").value = "";
      $("s-value").value = "";
      return load();
    }).catch(function (err) {
      message($("form-sent-msg"), "Not recorded: " + err.message, err.problems);
    });
  }

  function submitReturn(event) {
    event.preventDefault();
    var body = {
      no: $("b-no").value,
      date: $("b-date").value,
      againstChallanNo: $("b-against").value,
      qty: numberOrZero("b-qty"),
      itemDesc: $("b-desc").value
    };
    api("POST", "api/returns", body).then(function (data) {
      var text = "Return " + data.challan.no + " recorded.";
      if (data.warnings && data.warnings.length > 0) { text += " " + data.warnings.join(" "); }
      message($("form-return-msg"), text);
      $("b-no").value = "";
      $("b-qty").value = "";
      $("b-desc").value = "";
      return load();
    }).catch(function (err) {
      message($("form-return-msg"), "Not recorded: " + err.message, err.problems);
    });
  }

  function removeChallan(no, returnCount) {
    var warning = returnCount > 0
      ? "Remove challan " + no + " and the " + plural(returnCount, "return", "returns") + " recorded against it?"
      : "Remove " + no + " from the register?";
    if (!window.confirm(warning)) { return; }
    var url = "api/challans?no=" + encodeURIComponent(no) + (returnCount > 0 ? "&cascade=1" : "");
    api("DELETE", url).then(function () {
      banner("");
      return load();
    }).catch(function (err) {
      banner("Could not remove " + no + ": " + err.message);
    });
  }

  function submitSettings(event) {
    event.preventDefault();
    var body = {
      principalName: $("p-name").value,
      principalGstin: $("p-gstin").value,
      annualAggregateTurnoverInr: numberOrZero("p-turnover"),
      horizonDays: numberOrZero("p-horizon"),
      rules: {
        inputsReturnYears: numberOrZero("p-inputs-years"),
        capitalGoodsReturnYears: numberOrZero("p-capital-years"),
        halfYearlyTurnoverThresholdInr: numberOrZero("p-threshold"),
        filingDueDayOfMonth: numberOrZero("p-dueday")
      }
    };
    api("PUT", "api/settings", body).then(function () {
      message($("form-settings-msg"), "Saved.");
      $("horizon").value = "";
      return load();
    }).catch(function (err) {
      message($("form-settings-msg"), "Not saved: " + err.message, err.problems);
    });
  }

  function importFile() {
    var input = $("i-file");
    if (!input.files || input.files.length === 0) {
      message($("i-msg"), "Choose a CSV file first.");
      return;
    }
    var reader = new FileReader();
    reader.onerror = function () {
      message($("i-msg"), "That file could not be read.");
    };
    reader.onload = function () {
      api("POST", "api/import", String(reader.result), "text/csv").then(function (result) {
        var parts = [plural(result.added, "challan", "challans") + " brought in"];
        if (result.skipped > 0) { parts.push(result.skipped + " already in the register"); }
        var problems = (result.errors || []).concat(result.warnings || []);
        if (problems.length > 0) { parts.push(plural(problems.length, "row needs", "rows need") + " attention"); }
        message($("i-msg"), parts.join(" · ") + ".");

        var body = $("i-rows");
        clear(body);
        problems.sort(function (a, b) { return a.line - b.line; });
        problems.forEach(function (p) {
          var row = document.createElement("tr");
          row.appendChild(td(String(p.line), "num"));
          row.appendChild(td(p.message));
          body.appendChild(row);
        });
        show($("i-rows-table"), problems.length > 0);
        input.value = "";
        return load();
      }).catch(function (err) {
        message($("i-msg"), "Nothing was imported: " + err.message);
      });
    };
    reader.readAsText(input.files[0]);
  }

  /* ---------- panels and theme ---------- */

  function openPanel(name) {
    var buttons = $("tabs").querySelectorAll("button");
    var i;
    for (i = 0; i < buttons.length; i++) {
      var panel = buttons[i].getAttribute("data-panel");
      buttons[i].setAttribute("aria-pressed", panel === name ? "true" : "false");
      show($("panel-" + panel), panel === name);
    }
  }

  function wireTheme() {
    var root = document.documentElement;
    var button = document.querySelector("[data-themer]");
    if (!button) { return; }

    function stored() {
      try { return localStorage.getItem("omsme-theme"); } catch (e) { return null; }
    }
    function current() {
      var set = root.getAttribute("data-theme");
      if (set) { return set; }
      return (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) ? "dark" : "light";
    }
    function paint() {
      var dark = current() === "dark";
      button.textContent = dark ? "☀ Light" : "☾ Dark";
      button.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
    }
    paint();
    button.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem("omsme-theme", next); } catch (e) { /* private browsing */ }
      paint();
    });
    if (window.matchMedia) {
      window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
        if (!stored()) { paint(); }
      });
    }
  }

  function wire() {
    wireTheme();

    $("tabs").addEventListener("click", function (event) {
      var panel = event.target.getAttribute && event.target.getAttribute("data-panel");
      if (panel) { openPanel(panel); }
    });

    $("as-of").addEventListener("change", load);
    $("horizon").addEventListener("change", load);

    $("form-sent").addEventListener("submit", submitSent);
    $("form-return").addEventListener("submit", submitReturn);
    $("form-settings").addEventListener("submit", submitSettings);
    $("b-against").addEventListener("change", onAgainstChange);

    $("f-freq").addEventListener("click", function (event) {
      var chosen = event.target.getAttribute && event.target.getAttribute("data-freq");
      if (!chosen) { return; }
      frequency = chosen;
      renderPeriods();
    });
    $("f-period").addEventListener("change", updateDownloadLinks);
    $("f-asof").addEventListener("change", updateDownloadLinks);
    $("f-prepare").addEventListener("click", prepareFiling);

    $("i-go").addEventListener("click", importFile);
  }

  wire();
  load().then(function () {
    if (state && state.suggestedFrequency) {
      frequency = state.suggestedFrequency;
      renderPeriods();
    }
  });
})();
