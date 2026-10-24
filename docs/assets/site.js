// Small enhancements for the static docs: theme toggle, mobile menu,
// active nav link, heading anchors, copy buttons and XML highlighting.
(function () {
  "use strict";

  var root = document.documentElement;

  function storedTheme() {
    try { return localStorage.getItem("xml2pdf-theme"); } catch (e) { return null; }
  }
  function storeTheme(t) {
    try { localStorage.setItem("xml2pdf-theme", t); } catch (e) { /* ignore */ }
  }
  var initial = storedTheme();
  if (initial === "light" || initial === "dark") root.setAttribute("data-theme", initial);

  function escapeHtml(s) {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function markVars(html) {
    return html.replace(/(\$\{[^}]*\}|\{pages?\})/g, '<span class="tk-var">$1</span>');
  }

  // Minimal XML highlighter: comments, tags, attributes, strings, ${vars}.
  function highlightXml(src) {
    var out = "";
    var re = /<!--[\s\S]*?-->|<\?[\s\S]*?\?>|<\/?[A-Za-z][\w:-]*(?:\s+[\w:-]+(?:\s*=\s*(?:"[^"]*"|'[^']*'))?)*\s*\/?>/g;
    var last = 0, m;
    while ((m = re.exec(src)) !== null) {
      out += markVars(escapeHtml(src.slice(last, m.index)));
      var t = m[0];
      if (t.indexOf("<!--") === 0 || t.indexOf("<?") === 0) {
        out += '<span class="tk-com">' + escapeHtml(t) + "</span>";
      } else {
        var tm = /^(<\/?)([\w:-]+)([\s\S]*?)(\/?>)$/.exec(t);
        var attrs = escapeHtml(tm[3]).replace(
          /([\w:-]+)(\s*=\s*)(&quot;|")([^"]*)(")/g,
          function (_, name, eq, q1, val, q2) {
            return '<span class="tk-attr">' + name + "</span>" + eq +
              '<span class="tk-str">"' + markVars(val) + '"</span>';
          });
        out += '<span class="tk-tag">' + escapeHtml(tm[1]) + tm[2] + "</span>" + attrs +
          '<span class="tk-tag">' + escapeHtml(tm[4]) + "</span>";
      }
      last = re.lastIndex;
    }
    out += markVars(escapeHtml(src.slice(last)));
    return out;
  }

  function highlightShell(src) {
    return escapeHtml(src).split("\n").map(function (line) {
      var i = line.indexOf("#");
      if (i >= 0 && (i === 0 || /\s/.test(line[i - 1]))) {
        return line.slice(0, i) + '<span class="tk-com">' + line.slice(i) + "</span>";
      }
      return line;
    }).join("\n");
  }

  function highlightGo(src) {
    return escapeHtml(src)
      .replace(/(&quot;|")((?:[^"\\\n]|\\.)*)"/g, '<span class="tk-str">"$2"</span>')
      .replace(/(^|\s)(\/\/.*)$/gm, '$1<span class="tk-com">$2</span>')
      .replace(/\b(package|import|func|return|if|err|nil|var|type|struct|map|string|any|error)\b/g,
        '<span class="tk-tag">$1</span>');
  }

  document.addEventListener("DOMContentLoaded", function () {
    // Theme toggle.
    var toggle = document.getElementById("theme-toggle");
    if (toggle) {
      toggle.addEventListener("click", function () {
        var dark = root.getAttribute("data-theme") === "dark" ||
          (!root.getAttribute("data-theme") && window.matchMedia("(prefers-color-scheme: dark)").matches);
        var next = dark ? "light" : "dark";
        root.setAttribute("data-theme", next);
        storeTheme(next);
      });
    }

    // Mobile menu.
    var menu = document.getElementById("menu-btn");
    if (menu) {
      menu.addEventListener("click", function () { document.body.classList.toggle("nav-open"); });
      document.querySelectorAll(".sidebar a").forEach(function (a) {
        a.addEventListener("click", function () { document.body.classList.remove("nav-open"); });
      });
    }

    // Active page link.
    var page = location.pathname.split("/").pop() || "index.html";
    document.querySelectorAll(".sidebar > ul > li > a").forEach(function (a) {
      if (a.getAttribute("href") === page) a.classList.add("active");
    });

    // Heading anchors.
    document.querySelectorAll("main h2[id], main h3[id]").forEach(function (h) {
      var a = document.createElement("a");
      a.className = "anchor";
      a.href = "#" + h.id;
      a.textContent = "#";
      a.setAttribute("aria-label", "Link to this section");
      h.appendChild(a);
    });

    // Highlighting and copy buttons.
    document.querySelectorAll("pre > code").forEach(function (code) {
      var src = code.textContent;
      if (code.classList.contains("lang-xml")) code.innerHTML = highlightXml(src);
      else if (code.classList.contains("lang-sh")) code.innerHTML = highlightShell(src);
      else if (code.classList.contains("lang-go")) code.innerHTML = highlightGo(src);

      var pre = code.parentNode;
      var wrap = document.createElement("div");
      wrap.className = "code";
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);
      var btn = document.createElement("button");
      btn.className = "copy";
      btn.type = "button";
      btn.textContent = "Copy";
      btn.addEventListener("click", function () {
        var done = function () {
          btn.textContent = "Copied";
          setTimeout(function () { btn.textContent = "Copy"; }, 1200);
        };
        if (navigator.clipboard) navigator.clipboard.writeText(src).then(done, function () {});
      });
      wrap.appendChild(btn);
    });
  });
})();
