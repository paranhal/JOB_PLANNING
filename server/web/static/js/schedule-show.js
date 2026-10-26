(function () {
  var KEY = "jp.schedule.show";
  function ready() {
    var el = document.getElementById("schedule-show");
    if (!el) return;
    var url = new URL(location.href);
    var q = (url.searchParams.get("show") || "").trim();
    if (!q) {
      var saved = (localStorage.getItem(KEY) || "").trim();
      if (saved && saved !== "plan") {
        url.searchParams.set("show", saved);
        location.replace(url.toString());
        return;
      }
    } else {
      localStorage.setItem(KEY, q);
    }
    el.querySelectorAll("a[data-show-bit]").forEach(function (a) {
      a.addEventListener("click", function () {
        var href = a.getAttribute("href") || "";
        try {
          var u = new URL(href, location.origin);
          var s = u.searchParams.get("show") || "plan";
          localStorage.setItem(KEY, s);
        } catch (e) {}
      });
    });
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", ready);
  } else {
    ready();
  }
})();
