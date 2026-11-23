// 95-219 업종 다중 선택: 드롭다운 + 항목별 체크박스. 고객 등록·수정에서 쓰고, 다른 화면에서도 같은 컴포넌트를 재사용한다.
// 선택값은 name="industry" 체크 값 여러 개로 제출한다(서버가 고정 순서 쉼표 문자열로 저장한다).
(function (root) {
  function industryMulti(cfg) {
    cfg = cfg || {};
    return {
      id: cfg.id || 'industry-multi',
      name: cfg.name || 'industry',
      placeholder: cfg.placeholder || '선택하세요',
      all: (cfg.options || []).map(function (v) { return { value: v, label: v, extra: false }; })
        .concat((cfg.extras || []).map(function (v) { return { value: v, label: v, extra: true }; })),
      sel: (cfg.selected || []).slice(),
      open: false,
      active: -1,

      has: function (v) { return this.sel.indexOf(v) >= 0; },
      // 고정 순서(선택지 순서 → 기타 기존 값)로 정렬해 보관한다.
      sorted: function () {
        var order = this.all.map(function (o) { return o.value; });
        var self = this;
        return order.filter(function (v) { return self.sel.indexOf(v) >= 0; });
      },
      toggleValue: function (v) {
        var i = this.sel.indexOf(v);
        if (i >= 0) this.sel.splice(i, 1); else this.sel.push(v);
        this.sel = this.sorted();
      },
      // 닫힌 필드의 요약: 없으면 안내, 1~2개는 쉼표, 3개 이상은 「첫 항목 외 N」.
      summary: function () {
        var s = this.sorted();
        if (!s.length) return this.placeholder;
        if (s.length <= 2) return s.join(', ');
        return s[0] + ' 외 ' + (s.length - 1);
      },
      boxes: function () { return Array.prototype.slice.call(this.$refs.panel.querySelectorAll('input[type=checkbox]')); },
      show: function () {
        this.open = true;
        var self = this;
        this.$nextTick(function () {
          // 좁은 화면에서는 패널이 하단 탭바에 가리지 않게 필드를 화면 가운데로 올린다.
          if (typeof window !== 'undefined' && window.innerWidth < 768 && self.$refs.btn && self.$refs.btn.scrollIntoView) {
            self.$refs.btn.scrollIntoView({ block: 'center' });
          }
          var b = self.boxes();
          var first = b.find(function (x) { return x.checked; }) || b[0];
          if (first) first.focus();
        });
      },
      hide: function (focusBtn) {
        if (!this.open) return;
        this.open = false;
        if (focusBtn && this.$refs.btn) this.$refs.btn.focus();
      },
      toggle: function () { if (this.open) this.hide(false); else this.show(); },
      move: function (dir, ev) {
        var b = this.boxes();
        if (!b.length) return;
        var i = b.indexOf(document.activeElement);
        var n = i < 0 ? (dir > 0 ? 0 : b.length - 1) : (i + dir + b.length) % b.length;
        b[n].focus();
        if (ev && ev.preventDefault) ev.preventDefault();
      },
      // 키보드(Tab)로 포커스가 컴포넌트 밖의 다른 요소로 나가면 닫는다. 항목 줄을 마우스로 누를 때는 포커스가 잠깐 body 로
      // 가므로(relatedTarget 없음) 닫지 않는다. 바깥 클릭은 @click.outside 가 처리한다.
      focusOut: function (ev) {
        var root = this.$el;
        if (!ev.relatedTarget || root.contains(ev.relatedTarget)) return;
        this.open = false;
      }
    };
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = { industryMulti: industryMulti };
  root.industryMulti = industryMulti;
})(typeof window !== 'undefined' ? window : globalThis);
