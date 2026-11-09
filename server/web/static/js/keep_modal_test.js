const km = require('./keep_modal.js');

function fakeForm() {
  const btns = [{ disabled: false }, { disabled: false }];
  return {
    dataset: {},
    querySelectorAll: function () { return btns; },
    _btns: btns
  };
}

const a = fakeForm();
if (!km.tryLockSubmit(a)) throw new Error('첫 잠금 실패');
if (a.dataset.submitting !== '1') throw new Error('submitting 미설정');
if (!a._btns[0].disabled) throw new Error('버튼 미잠금');
if (km.tryLockSubmit(a)) throw new Error('이중 제출이 통과');
km.unlockSubmit(a);
if (a.dataset.submitting) throw new Error('잠금 해제 실패');
if (a._btns[0].disabled) throw new Error('버튼 미해제');
console.log('keep_modal_test.js ok');
