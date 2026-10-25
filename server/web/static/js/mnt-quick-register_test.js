const fs = require('fs');
const path = require('path');
const src = fs.readFileSync(path.join(__dirname, 'mnt-quick-register.js'), 'utf8');
for (const s of ['/workboard/mnt-quick-register', 'bindMntQuickRegister', 'X-CSRF-Token', 'mnt-register-source', 'visit_id']) {
  if (!src.includes(s)) {
    console.error('missing', s);
    process.exit(1);
  }
}
console.log('ok');
