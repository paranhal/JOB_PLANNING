(function () {
  var el = document.getElementById("wb-work-groups");
  if (el) {
    try { window.assigneePickerGroups = JSON.parse(el.textContent || "[]"); } catch (e) {}
  }
})();

window.assigneePicker = window.assigneePicker || function (cfg) {
  cfg = cfg || {};
  return {
    open: false, q: '', groups: {}, extra: {}, exclude: {}, expanded: {}, ownerId: '',
    supportMin: {}, external: '',
    target: cfg.target || 'add',
    mode: cfg.mode || 'multi',
    allowExternal: !!cfg.allowExternal,
    allowGroups: cfg.allowGroups !== false,
    allGroups() { return cfg.groups || window.mntWorkGroups || window.assigneePickerGroups || []; },
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
      if (this.mode === 'single' && ids.length > 1) return [this.ownerId || ids[0]];
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
      if (el.getAttribute('data-name')) return el.getAttribute('data-name');
      return lab ? String(lab.textContent || '').trim() : id;
    },
    leaveNames() {
      try {
        const p = this.$parent || {};
        return p.leaveNames || p.DayLeaveNames || window.dayLeaveNames || [];
      } catch (e) { return []; }
    },
    confirmLeave(id) {
      const name = this.nameOf(id);
      const leaves = this.leaveNames() || [];
      if (!name || !leaves.length) return true;
      if (leaves.indexOf(name) < 0) return true;
      return window.confirm(name + ' 님은 연차입니다. 그래도 넣을까요?');
    },
    summaryLabel() {
      const gids = this.selectedGroupIds();
      const ids = this.selectedIds();
      if (!ids.length && !gids.length && !this.external) return '— 선택 —';
      if (this.external && !ids.length) return this.external;
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
      if (this.mode === 'single') {
        this.clear();
      }
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
      if (!this.confirmLeave(uid) && on) return;
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
      if (on && !this.confirmLeave(id)) return;
      if (this.mode === 'single' && on) {
        this.clear();
      }
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
    clear() { this.groups = {}; this.extra = {}; this.exclude = {}; this.ownerId = ''; this.supportMin = {}; },
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
    seedFromParent() {
      try {
        const p = this.$parent || {};
        const name = (p.preAssignee || '').trim();
        if (!name) return;
        const el = this.$el.querySelector('input[type=checkbox][data-name="' + name + '"]');
        if (el && el.value) this.togglePerson(el.value, true);
      } catch (e) {}
    },
    hydrateFrom(d) {
      if (!d || (d.target && d.target !== this.target)) return;
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
      if (d.external) this.external = d.external;
    }
  };
};

window.mntAssigneeMulti = window.mntAssigneeMulti || window.assigneePicker;
