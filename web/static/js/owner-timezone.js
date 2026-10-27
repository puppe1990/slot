// Preselect the browser timezone on /setup. The owner account starts as UTC,
// so a consultant in São Paulo used to save 09:00-17:00 meaning UTC and see
// every slot three hours off.
//
// Loaded by the layout, so it also runs after a Drive morph (#325): the page
// script would only fire on a full load.
(function () {
  if (window.__slotOwnerTimezone) return;
  window.__slotOwnerTimezone = true;

  function detect() {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch (e) {
      return "";
    }
  }

  function ensureOption(select, zone) {
    var option = Array.prototype.find.call(select.options, function (o) {
      return o.value === zone;
    });
    if (option) return option;
    option = document.createElement("option");
    option.value = zone;
    option.textContent = zone;
    select.appendChild(option);
    return option;
  }

  function init() {
    var select = document.getElementById("owner-timezone");
    var hint = document.getElementById("tz-hint");
    var label = document.getElementById("tz-detected");
    var use = document.getElementById("tz-use");
    if (!select || !hint || !label || !use) return;

    var zone = detect();
    if (!zone) return;

    ensureOption(select, zone);
    label.textContent = zone;

    if (select.dataset.firstVisit === "true") {
      // Nothing saved yet: apply it and say so. No "Use" button, it is done.
      select.value = zone;
      hint.classList.remove("hidden");
      use.classList.add("hidden");
      return;
    }

    if (select.value === zone) {
      hint.classList.add("hidden");
      return;
    }

    hint.classList.remove("hidden");
    use.classList.remove("hidden");

    // Plain assignment: init runs again after a morph, and addEventListener
    // would stack handlers on the reused node.
    use.onclick = function () {
      select.value = zone;
      select.focus();
      hint.classList.add("hidden");
    };
  }

  document.addEventListener("DOMContentLoaded", init);
  document.addEventListener("amarra:morphed", init);
  window.addEventListener("popstate", init);
  init();
})();
