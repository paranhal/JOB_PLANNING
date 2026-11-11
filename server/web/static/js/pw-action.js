(function (w) {
  function pad(n) { return n < 10 ? '0' + n : String(n); }
  function parseLocal(s) {
    s = String(s || '').trim().replace(' ', 'T');
    if (s.length < 16) return null;
    var d = new Date(s.slice(0, 16));
    return isNaN(d.getTime()) ? null : d;
  }
  function fmtLocal(d) {
    if (!d) return '';
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) +
      'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }
  function addMin(s, min) {
    var d = parseLocal(s);
    if (!d) return '';
    d.setMinutes(d.getMinutes() + (parseInt(min, 10) || 0));
    return fmtLocal(d);
  }
  function diffMin(a, b) {
    var x = parseLocal(a), y = parseLocal(b);
    if (!x || !y) return 0;
    return Math.round((y.getTime() - x.getTime()) / 60000);
  }
  var SITE_CHO = 'ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ';
  function siteNorm(s) {
    return String(s || '').toLowerCase().replace(/\s+/g, '');
  }
  function siteIsCho(ch) {
    return SITE_CHO.indexOf(ch) >= 0;
  }
  function siteChoOf(ch) {
    var c = ch.charCodeAt(0);
    if (c >= 0xAC00 && c <= 0xD7A3) return SITE_CHO.charAt(Math.floor((c - 0xAC00) / 588));
    return '';
  }
  function siteChoSung(s) {
    var out = '';
    for (var i = 0; i < s.length; i++) {
      var ch = siteChoOf(s.charAt(i));
      if (ch) out += ch;
    }
    return out;
  }
  function siteCache(site) {
    if (site._blob != null) return site;
    site._blob = siteNorm(String(site.name || '') + String(site.parent || '') + String(site.region || ''));
    site._cho = siteChoSung(site._blob);
    return site;
  }
  function siteQueryKind(q) {
    var all = q.length > 0;
    var any = false;
    for (var i = 0; i < q.length; i++) {
      if (siteIsCho(q.charAt(i))) any = true;
      else all = false;
    }
    return all ? 'cho' : (any ? 'mix' : 'text');
  }
  function siteMixMatch(hay, q) {
    if (q.length > hay.length) return false;
    var last = hay.length - q.length;
    for (var i = 0; i <= last; i++) {
      var ok = true;
      for (var j = 0; j < q.length; j++) {
        var qc = q.charAt(j);
        var hc = hay.charAt(i + j);
        if (siteIsCho(qc)) {
          if (siteChoOf(hc) !== qc && hc !== qc) { ok = false; break; }
        } else if (hc !== qc) { ok = false; break; }
      }
      if (ok) return true;
    }
    return false;
  }
  function siteMatch(site, q, region) {
    site = siteCache(site || {});
    var want = String(region || '').trim();
    if (want && String(site.region || '') !== want) return false;
    q = siteNorm(q);
    if (!q) return true;
    var kind = siteQueryKind(q);
    if (kind === 'cho') return site._cho.indexOf(q) >= 0;
    if (kind === 'mix') return siteMixMatch(site._blob, q);
    return site._blob.indexOf(q) >= 0;
  }
  w.pwSiteNorm = siteNorm;
  w.pwSiteImportMatch = siteMatch;
  w.pwSiteImport = function (raw) {
    var sites = [];
    var seenRegion = {};
    var regions = [];
    (raw || []).forEach(function (s) {
      var it = {
        id: String(s.id || ''),
        name: String(s.name || ''),
        parent: String(s.parent || ''),
        region: String(s.region || '')
      };
      if (!it.id) return;
      siteCache(it);
      sites.push(it);
      if (it.region && !seenRegion[it.region]) {
        seenRegion[it.region] = true;
        regions.push(it.region);
      }
    });
    regions.sort();
    return {
      sites: sites,
      regions: regions,
      q: '',
      region: '',
      picked: {},
      match: function (s) { return siteMatch(s, this.q, this.region); },
      visible: function () {
        var out = [];
        for (var i = 0; i < this.sites.length; i++) {
          if (this.match(this.sites[i])) out.push(this.sites[i]);
        }
        return out;
      },
      visibleCount: function () { return this.visible().length; },
      pickedCount: function () {
        var n = 0;
        for (var k in this.picked) if (this.picked[k]) n++;
        return n;
      },
      pickedSites: function () {
        var out = [];
        for (var i = 0; i < this.sites.length; i++) {
          if (this.picked[this.sites[i].id]) out.push(this.sites[i]);
        }
        return out;
      },
      setPicked: function (id, on) {
        var next = {};
        for (var k in this.picked) if (this.picked[k]) next[k] = true;
        if (on) next[id] = true;
        else delete next[id];
        this.picked = next;
      },
      toggle: function (id, on) { this.setPicked(id, !!on); },
      selectVisible: function () {
        var next = {};
        for (var k in this.picked) if (this.picked[k]) next[k] = true;
        var rows = this.visible();
        for (var i = 0; i < rows.length; i++) next[rows[i].id] = true;
        this.picked = next;
      },
      clearVisible: function () {
        var next = {};
        for (var k in this.picked) if (this.picked[k]) next[k] = true;
        var rows = this.visible();
        for (var i = 0; i < rows.length; i++) delete next[rows[i].id];
        this.picked = next;
      },
      clearAll: function () { this.picked = {}; },
      unpick: function (id) { this.setPicked(id, false); },
      label: function (s) {
        var t = s.name || '';
        if (s.parent) t += ' · ' + s.parent;
        if (s.region) t += ' · ' + s.region;
        return t;
      },
      onSearchKey: function (ev) {
        if (ev && ev.key === 'Enter') {
          ev.preventDefault();
          ev.stopPropagation();
        }
      }
    };
  };
  w.pwAddMinutes = addMin;
  w.pwDiffMinutes = diffMin;
  w.pwActionScreen = function (cfg) {
    cfg = cfg || {};
    return {
      subtype: cfg.subtype || '',
      place: cfg.place || '',
      start: cfg.start || '',
      duration: String(cfg.duration || 30),
      durationCustom: cfg.durationCustom || '',
      complete: cfg.complete || '',
      completeTouched: !!cfg.completeTouched,
      result: cfg.result || 'done',
      supports: cfg.supports || [],
      pickId: '',
      customName: '',
      leaveOpen: false,
      saving: false,
      infoOpen: false,
      dirty: false,
      submitLabel() {
        if (this.result === 'partial') return '추가조치 등록';
        if (this.result === 'transfer') return '이관 처리';
        return '조치 완료';
      },
      durValue() {
        if (this.duration === 'custom') return parseInt(this.durationCustom, 10) || 0;
        return parseInt(this.duration, 10) || 0;
      },
      syncCompleteFromStart() {
        if (this.completeTouched) return;
        var m = this.durValue();
        if (!this.start || m <= 0) return;
        this.complete = addMin(this.start, m);
      },
      onStartChange() {
        this.dirty = true;
        this.syncCompleteFromStart();
      },
      onDurationChange() {
        this.dirty = true;
        if (this.completeTouched) {
          var m = diffMin(this.start, this.complete);
          if (m > 0) {
            if ([30, 60, 90, 120, 180, 240, 480].indexOf(m) >= 0) {
              this.duration = String(m);
            } else {
              this.duration = 'custom';
              this.durationCustom = String(m);
            }
          }
          return;
        }
        this.syncCompleteFromStart();
      },
      onCompleteChange() {
        this.dirty = true;
        this.completeTouched = true;
        var m = diffMin(this.start, this.complete);
        if (m > 0) {
          if ([30, 60, 90, 120, 180, 240, 480].indexOf(m) >= 0) {
            this.duration = String(m);
          } else {
            this.duration = 'custom';
            this.durationCustom = String(m);
          }
        }
      },
      addPicked() {
        if (!this.pickId) return;
        var opt = document.querySelector('#pw-support-pick option[value="' + this.pickId + '"]');
        var name = opt ? (opt.getAttribute('data-name') || opt.textContent || '') : '';
        this.addSupport(this.pickId, name.trim());
        this.pickId = '';
      },
      addCustom() {
        var parts = String(this.customName || '').replace(/\n/g, ',').split(',');
        this.customName = '';
        for (var i = 0; i < parts.length; i++) {
          var n = parts[i].trim();
          if (n) this.addSupport('', n);
        }
      },
      addSupport(id, name) {
        id = String(id || '').trim();
        name = String(name || '').trim();
        if (!id && !name) return;
        var owner = document.getElementById('pw-assignee-code');
        var ownerId = owner ? String(owner.value || '') : '';
        if (id && ownerId && id === ownerId) {
          w.dispatchEvent(new CustomEvent('app-flash-error', { detail: '처리 담당자는 지원 인력에 넣을 수 없습니다' }));
          return;
        }
        for (var i = 0; i < this.supports.length; i++) {
          var s = this.supports[i];
          if (id && s.id === id) return;
          if (!id && s.name === name) return;
        }
        if (this.supports.length >= 20) {
          w.dispatchEvent(new CustomEvent('app-flash-error', { detail: '지원 인력은 최대 20명입니다' }));
          return;
        }
        this.supports.push({ id: id, name: name });
        this.dirty = true;
      },
      removeSupport(i) {
        this.supports.splice(i, 1);
        this.dirty = true;
      },
      onCancel(url) {
        if (this.dirty) { this.leaveOpen = true; this._leaveUrl = url; return; }
        w.location.href = url;
      },
      leave() {
        w.location.href = this._leaveUrl || '/';
      },
      onSubmit() { this.saving = true; }
    };
  };
})(window);
