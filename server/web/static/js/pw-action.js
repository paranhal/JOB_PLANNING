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
