# 코드관리 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QLabel, QListWidget, QListWidgetItem,
    QTableWidget, QTableWidgetItem, QGroupBox, QPushButton, QMessageBox,
    QInputDialog, QSplitter,
)
from PySide6.QtCore import Qt

from as_support.services import code_service
from as_support.config import CODE_GROUPS, CODE_GROUP_LABELS


class CodeMasterFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        code_service.ensure_defaults()
        self._current_group = None
        self._build_ui()
        self._refresh_groups()

    def _build_ui(self):
        layout = QVBoxLayout(self)
        split = QSplitter(Qt.Orientation.Horizontal)
        left_gb = QGroupBox("코드 그룹")
        left_layout = QVBoxLayout(left_gb)
        self._group_list = QListWidget()
        self._group_list.currentItemChanged.connect(self._on_group_select)
        left_layout.addWidget(self._group_list)
        split.addWidget(left_gb)

        right_gb = QGroupBox("코드 값")
        right_layout = QVBoxLayout(right_gb)
        self._value_table = QTableWidget()
        self._value_table.setColumnCount(3)
        self._value_table.setHorizontalHeaderLabels(["코드값", "표시명", "사용"])
        self._value_table.horizontalHeader().setStretchLastSection(True)
        right_layout.addWidget(self._value_table)
        btn_row = QHBoxLayout()
        btn_row.addWidget(QPushButton("값 추가", clicked=self._add_value))
        btn_row.addWidget(QPushButton("초기화(기본값)", clicked=self._ensure_defaults))
        right_layout.addLayout(btn_row)
        split.addWidget(right_gb)
        split.setSizes([200, 450])
        layout.addWidget(split)

    def _refresh_groups(self):
        self._group_list.clear()
        for g in CODE_GROUPS:
            self._group_list.addItem(CODE_GROUP_LABELS.get(g, g))
        if self._group_list.count():
            self._group_list.setCurrentRow(0)
            self._on_group_select(self._group_list.currentItem(), None)

    def _on_group_select(self, current, previous):
        if not current:
            return
        idx = self._group_list.row(current)
        self._current_group = CODE_GROUPS[idx]
        self._refresh_values()

    def _refresh_values(self):
        self._value_table.setRowCount(0)
        if not self._current_group:
            return
        from as_support.store import code_master
        for r in code_master.list_by_group(self._current_group, use_only=False):
            row = self._value_table.rowCount()
            self._value_table.insertRow(row)
            self._value_table.setItem(row, 0, QTableWidgetItem(r.get("code_value") or ""))
            self._value_table.setItem(row, 1, QTableWidgetItem(r.get("code_name") or ""))
            self._value_table.setItem(row, 2, QTableWidgetItem("Y" if r.get("use_yn", True) else "N"))

    def _add_value(self):
        if not self._current_group:
            return
        v, ok = QInputDialog.getText(self, "코드 추가", "코드값(표시명과 동일하게 저장):")
        if not ok or not v or not v.strip():
            return
        try:
            code_service.add_code(self._current_group, v.strip(), v.strip())
            QMessageBox.information(self, "완료", "추가되었습니다.")
            self._refresh_values()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _ensure_defaults(self):
        code_service.ensure_defaults()
        QMessageBox.information(self, "완료", "기본 코드가 반영되었습니다.")
        self._refresh_values()
