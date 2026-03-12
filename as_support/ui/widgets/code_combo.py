# 코드 콤보박스 — PySide6

from PySide6.QtWidgets import QComboBox

from as_support.store import code_master


class CodeCombo(QComboBox):
    def __init__(self, parent=None, code_group: str = "", use_only: bool = True):
        super().__init__(parent)
        self._code_group = code_group
        self._use_only = use_only
        self.setEditable(False)
        self._refresh()

    def _refresh(self):
        self.clear()
        rows = code_master.list_by_group(self._code_group, use_only=self._use_only)
        for r in rows:
            name = r.get("code_name") or r.get("code_value") or ""
            self.addItem(name, r.get("code_value"))
        if self.count() and not self.currentData():
            self.setCurrentIndex(0)

    def get_code_value(self):
        return self.currentData()

    def set_code_value(self, code_value):
        if not code_value:
            self.setCurrentIndex(-1)
            return
        for i in range(self.count()):
            if self.itemData(i) == code_value:
                self.setCurrentIndex(i)
                return
        # not in list (e.g. use_only=True filtered it)
        rows = code_master.list_by_group(self._code_group, use_only=False)
        for r in rows:
            if r.get("code_value") == code_value:
                self.addItem(r.get("code_name") or code_value, code_value)
                for i in range(self.count()):
                    if self.itemData(i) == code_value:
                        self.setCurrentIndex(i)
                        break
                return

    def refresh(self):
        self._refresh()
