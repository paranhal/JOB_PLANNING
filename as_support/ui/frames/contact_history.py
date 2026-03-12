# 담당자이력 조회 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QLabel, QTableWidget, QTableWidgetItem,
    QGroupBox, QComboBox,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service, contact_history_service
from as_support.store import contact as contact_store


class ContactHistoryFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._build_ui()
        self._refresh_customers()

    def _build_ui(self):
        layout = QVBoxLayout(self)
        top = QHBoxLayout()
        top.addWidget(QLabel("기관:"))
        self._customer_combo = QComboBox()
        self._customer_combo.setMinimumWidth(280)
        self._customer_combo.currentIndexChanged.connect(self._on_customer_change)
        top.addWidget(self._customer_combo)
        layout.addLayout(top)

        gb = QGroupBox("담당자 변경 이력")
        table_layout = QVBoxLayout(gb)
        self._table = QTableWidget()
        self._table.setColumnCount(6)
        self._table.setHorizontalHeaderLabels(["담당자", "시작일", "종료일", "담당업무", "상태", "변경사유"])
        self._table.horizontalHeader().setStretchLastSection(True)
        table_layout.addWidget(self._table)
        layout.addWidget(gb)

    def _refresh_customers(self):
        self._customer_combo.clear()
        for c in customer_service.list_all():
            self._customer_combo.addItem(c.get("name") or "", c.get("customer_id"))
        if self._customer_combo.count():
            self._customer_combo.setCurrentIndex(0)
            self._on_customer_change()

    def _on_customer_change(self):
        customer_id = self._customer_combo.currentData()
        self._table.setRowCount(0)
        if not customer_id:
            return
        contacts_by_id = {ct.get("contact_id"): ct.get("name") for ct in contact_store.load_all()}
        for h in contact_history_service.list_by_customer(customer_id):
            row = self._table.rowCount()
            self._table.insertRow(row)
            contact_name = contacts_by_id.get(h.get("contact_id"), "")
            self._table.setItem(row, 0, QTableWidgetItem(contact_name))
            self._table.setItem(row, 1, QTableWidgetItem(h.get("start_date") or ""))
            self._table.setItem(row, 2, QTableWidgetItem(h.get("end_date") or ""))
            self._table.setItem(row, 3, QTableWidgetItem(h.get("duty_code") or ""))
            self._table.setItem(row, 4, QTableWidgetItem(h.get("status_code") or ""))
            self._table.setItem(row, 5, QTableWidgetItem((h.get("change_reason") or "")[:50]))
