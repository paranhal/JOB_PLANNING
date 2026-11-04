window.mntAssigneeMulti = window.mntAssigneeMulti || function (cfg) {
  cfg = cfg || {};
  return {
    open: false, q: '', groups: {}, extra: {}, exclude: {}, expanded: {}, ownerId: '',
    target: cfg.target || 'add',
    allGroups() { return window.mntWorkGroups || []; },
    groupOf(id) { return this.allGroups().find((x) => x.id === id) || { members: [] }; },
    match(name) {
      const s = (this.q || '').trim().toLowerCase();
      if (!s) return true;
      return String(name || '').toLowerCase().indexOf(s) >= 0;
    },
    orgLabelOf(id) { return this.groupOf(id).org_label || ''; },
    memberSummaryOf(id) { return this.groupOf(id).member_summary || ''; },
    membersOf(id) { return this.groupOf(id).members || []; },
    groupVisible(id) {
      const g = this.groupOf(id);
      if (this.match(g.name)) return true;
      if (this.match(g.member_summary)) return true;
      return (g.members || []).some((m) => this.match(m.name));
    },
    selectedGroupIds() { return Object.keys(this.groups).filter((id) => this.groups[id]); },
    extraIds() { return Object.keys(this.extra).filter((id) => this.extra[id]); },
    excludeIds() { return Object.keys(this.exclude).filter((id) => this.exclude[id]); },
    inSelectedGroups(uid) {
      return this.selectedGroupIds().some((gid) => (this.membersOf(gid) || []).some((m) => m.user_id === uid));
    },
    selectedIds() {
      const seen = {};
      const ids = [];
      this.selectedGroupIds().forEach((gid) => {
        (this.membersOf(gid) || []).forEach((m) => {
          const id = m.user_id;
          if (!id || seen[id] || this.exclude[id]) return;
          seen[id] = true;
          ids.push(id);
        });
      });
      this.extraIds().forEach((id) => {
        if (!id || seen[id]) return;
        seen[id] = true;
        ids.push(id);
      });
      return ids;
    },
    personOn(id) { return this.selectedIds().indexOf(id) >= 0; },
    memberOn(gid, uid) { return !!this.groups[gid] && !this.exclude[uid]; },
    nameOf(id) {
      const gmem = this.allGroups().flatMap((g) => g.members || []);
      const hit = gmem.find((m) => m.user_id === id);
      if (hit && hit.name) return hit.name;
      const el = this.$el.querySelector('input[type=checkbox][value="' + id + '"]');
      if (!el) return id;
      const lab = el.closest('label');
      return lab ? String(lab.textContent || '').trim() : id;
    },
    summaryLabel() {
      const gids = this.selectedGroupIds();
      const ids = this.selectedIds();
      if (!ids.length && !gids.length) return '— 선택 —';
      const extras = this.extraIds().filter((id) => !this.inSelectedGroups(id));
      if (gids.length === 1 && extras.length === 0 && this.excludeIds().length === 0) {
        return this.groupOf(gids[0]).name || '그룹';
      }
      if (!gids.length && ids.length === 1) return this.nameOf(ids[0]);
      const head = gids.length ? (this.groupOf(gids[0]).name || this.nameOf(ids[0])) : this.nameOf(ids[0]);
      if (extras.length) {
        return head + ' 외 ' + this.nameOf(extras[0]) + ' 등 ' + ids.length + '명';
      }
      if (ids.length <= 1) return head;
      return head + ' 외 ' + (ids.length - 1) + '명';
    },
    toggleGroup(gid, on) {
      if (on) {
        this.groups[gid] = true;
        (this.membersOf(gid) || []).forEach((m) => { delete this.exclude[m.user_id]; delete this.extra[m.user_id]; });
      } else {
        delete this.groups[gid];
        (this.membersOf(gid) || []).forEach((m) => { delete this.exclude[m.user_id]; });
      }
      this.syncOwner();
    },
    toggleGroupMember(gid, uid, on) {
      if (!this.groups[gid]) {
        if (!on) return;
        this.groups[gid] = true;
        (this.membersOf(gid) || []).forEach((m) => {
          if (m.user_id !== uid) this.exclude[m.user_id] = true;
          else delete this.exclude[uid];
        });
      } else if (on) {
        delete this.exclude[uid];
      } else {
        this.exclude[uid] = true;
        const mems = this.membersOf(gid) || [];
        if (mems.length && mems.every((m) => this.exclude[m.user_id])) {
          delete this.groups[gid];
          mems.forEach((m) => delete this.exclude[m.user_id]);
        }
      }
      this.syncOwner();
    },
    togglePerson(id, on) {
      if (this.inSelectedGroups(id)) {
        if (on) delete this.exclude[id];
        else this.exclude[id] = true;
      } else if (on) {
        this.extra[id] = true;
      } else {
        delete this.extra[id];
      }
      this.syncOwner();
    },
    clear() { this.groups = {}; this.extra = {}; this.exclude = {}; this.ownerId = ''; },
    syncOwner() {
      const ids = this.selectedIds();
      const gids = this.selectedGroupIds();
      if (gids.length) {
        const g = this.groupOf(gids[0]);
        if (g.leader_id && ids.indexOf(g.leader_id) >= 0) {
          this.ownerId = g.leader_id;
          return;
        }
      }
      if (this.ownerId && ids.indexOf(this.ownerId) >= 0) return;
      this.ownerId = ids[0] || '';
    },
    hydrateFrom(d) {
      if (!d || d.target !== this.target) return;
      this.groups = {};
      this.extra = {};
      this.exclude = {};
      const saved = {};
      (d.ids || []).forEach((id) => { if (id) saved[id] = true; });
      String(d.groups || d.group || '').split(',').map((s) => s.trim()).filter(Boolean).forEach((gid) => {
        this.groups[gid] = true;
        (this.membersOf(gid) || []).forEach((m) => {
          if (m.user_id && !saved[m.user_id]) this.exclude[m.user_id] = true;
        });
      });
      (d.ids || []).forEach((id) => {
        if (id && !this.inSelectedGroups(id)) this.extra[id] = true;
      });
      this.ownerId = d.owner || this.selectedIds()[0] || '';
    }
  };
};

window.mntVisitAction = function (cfg) {
  cfg = cfg || {};
  return {
    start: cfg.start || '',
    end: cfg.end || '',
    travel: cfg.travel || 0,
    duration: cfg.duration || 0,
    planStart: cfg.planStart || '',
    planEnd: cfg.planEnd || '',
    q: '',
    productFilter: '',
    customs: cfg.customs || [],
    selected: cfg.selected || {},
    warn: '',
    toMin(v) {
      if (!v) return null;
      var p = String(v).split(':');
      if (p.length < 2) return null;
      var h = +p[0], m = +p[1];
      if (isNaN(h) || isNaN(m)) return null;
      return h * 60 + m;
    },
    fmtMin(m) {
      if (m == null || isNaN(m) || m <= 0) return '—';
      var h = Math.floor(m / 60), r = m % 60;
      return (h ? h + '시간 ' : '') + (r || !h ? r + '분' : '');
    },
    fmtT(m) {
      var h = Math.floor(m / 60) % 24, r = m % 60;
      return (h < 10 ? '0' : '') + h + ':' + (r < 10 ? '0' : '') + r;
    },
    siteMin() {
      var s = this.toMin(this.start), e = this.toMin(this.end);
      if (s == null || e == null) return null;
      if (e <= s) return null;
      return e - s;
    },
    calc() {
      var s = this.toMin(this.start), e = this.toMin(this.end);
      this.warn = '';
      if (s != null && e != null && e <= s) {
        this.warn = '종료 시각이 방문 시각보다 빠르거나 같습니다.';
      }
      var d = this.siteMin();
      if (d != null) this.duration = d;
      return !this.warn;
    },
    nowTo(which) {
      var n = new Date();
      var v = this.fmtT(n.getHours() * 60 + n.getMinutes());
      if (which === 'end') this.end = v; else this.start = v;
      this.calc();
    },
    fillPlan() {
      if (this.planStart) this.start = this.planStart;
      if (this.planEnd) this.end = this.planEnd;
      this.calc();
    },
    applyChip(min) {
      var s = this.toMin(this.start);
      if (s == null) {
        this.warn = '먼저 방문(도착) 시각을 입력하세요.';
        window.dispatchEvent(new CustomEvent('app-flash-error', { detail: this.warn }));
        return;
      }
      this.end = this.fmtT(s + (+min));
      this.calc();
    },
    durLabel() { return this.fmtMin(this.siteMin() != null ? this.siteMin() : this.duration); },
    totLabel() {
      var d = this.siteMin();
      if (d == null && this.duration > 0) d = this.duration;
      if (d == null) return '—';
      return this.fmtMin(d + (parseInt(this.travel, 10) || 0) * 2);
    },
    chipOn(min) {
      var d = this.siteMin();
      return d != null && d === +min;
    },
    addCustom() {
      if (this.customs.length >= 50) return;
      this.customs.push({ name: '', memo: '' });
    },
    removeCustom(i) { this.customs.splice(i, 1); },
    assetOn(id) { return !!this.selected[id]; },
    toggleAsset(id, on) {
      if (on) this.selected[id] = true;
      else delete this.selected[id];
    },
    selectAll(ids, on) {
      (ids || []).forEach((id) => { if (on) this.selected[id] = true; else delete this.selected[id]; });
    },
    matchAsset(name, type) {
      var s = (this.q || '').trim().toLowerCase();
      var p = (this.productFilter || '').trim().toLowerCase();
      if (s && String(name || '').toLowerCase().indexOf(s) < 0 && String(type || '').toLowerCase().indexOf(s) < 0) return false;
      if (p && String(type || '').toLowerCase().indexOf(p) < 0 && String(name || '').toLowerCase().indexOf(p) < 0) return false;
      return true;
    },
    onSubmit(e) {
      if (!this.calc()) {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('app-flash-error', { detail: this.warn + ' 시간을 확인해 주세요.' }));
        return false;
      }
      return true;
    }
  };
};

window.mntActionDialog = function (cfg) {
  cfg = cfg || {};
  return {
    open: true,
    back: cfg.back || '',
    embed: !!cfg.embed,
    done: !!cfg.done,
    init() {
      var self = this;
      this.$nextTick(function () { if (self.$refs.ok) self.$refs.ok.focus(); });
    },
    close(goBack) {
      this.open = false;
      if (this.embed && window.parent && window.parent !== window) {
        try { window.parent.postMessage({ type: 'jp-mnt-action', done: this.done }, '*'); } catch (e) {}
      }
      if (goBack && this.back) {
        window.location.href = this.back;
      }
    }
  };
};
