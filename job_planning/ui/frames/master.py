# -*- coding: utf-8 -*-
"""통합 고객·담당자·장비 마스터 — 구현_탭_파일구조_설계.md §4."""
from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QGridLayout, QGroupBox,
    QLabel, QLineEdit, QComboBox, QPushButton, QTableWidget,
    QTableWidgetItem, QHeaderView, QMessageBox, QAbstractItemView,
    QTabWidget,
)
from PySide6.QtCore import Qt

from ... import config
from ...services import customer_service
from ...services import contact_service
from ...services import equipment_service


class CustomerTab(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._selected_id = None
        layout = QVBoxLayout(self)
        g = QGroupBox("고객 목록")
        g_layout = QVBoxLayout(g)
        filter_layout = QHBoxLayout()
        filter_layout.addWidget(QLabel("구분 필터:"))
        self._filter_division_combo = QComboBox()
        self._filter_division_combo.addItem("전체", None)
        self._filter_division_combo.setMinimumWidth(120)
        filter_layout.addWidget(self._filter_division_combo)
        filter_layout.addStretch()
        g_layout.addLayout(filter_layout)
        self._table = QTableWidget()
        self._table.setColumnCount(4)
        self._table.setHorizontalHeaderLabels(["ID", "고객명", "구분", "연락처"])
        self._table.horizontalHeader().setSectionResizeMode(1, QHeaderView.Stretch)
        self._table.setSelectionBehavior(QAbstractItemView.SelectRows)
        self._table.itemSelectionChanged.connect(self._on_select)
        g_layout.addWidget(self._table)
        btn = QHBoxLayout()
        btn.addWidget(QPushButton("추가", clicked=self._on_add))
        btn.addWidget(QPushButton("수정", clicked=self._on_edit))
        btn.addWidget(QPushButton("삭제", clicked=self._on_delete))
        g_layout.addLayout(btn)
        layout.addWidget(g)
        self._form = QGroupBox("고객 입력/수정")
        f_layout = QGridLayout(self._form)
        f_layout.addWidget(QLabel("고객명:"), 0, 0)
        self._name_edit = QLineEdit()
        f_layout.addWidget(self._name_edit, 0, 1)
        f_layout.addWidget(QLabel("구분:"), 0, 2)
        self._division_edit = QLineEdit()
        self._division_edit.setPlaceholderText("NICOM / K-LAS 등")
        f_layout.addWidget(self._division_edit, 0, 3)
        f_layout.addWidget(QLabel("연락처/비고:"), 1, 0)
        self._phone_edit = QLineEdit()
        f_layout.addWidget(self._phone_edit, 1, 1, 1, 3)
        layout.addWidget(self._form)
        self._filter_division_combo.currentIndexChanged.connect(self.load_list)
        self._divisions_loaded = False

    def _on_select(self):
        row = self._table.currentRow()
        if row < 0:
            self._selected_id = None
            return
        it = self._table.item(row, 0)
        self._selected_id = int(it.text()) if it else None

    def load_list(self):
        if not getattr(self, "_divisions_loaded", False):
            self._divisions_loaded = True
            all_cust = customer_service.list_()
            divisions = sorted({r.get("division") for r in all_cust if r.get("division")})
            self._filter_division_combo.blockSignals(True)
            self._filter_division_combo.clear()
            self._filter_division_combo.addItem("전체", None)
            for d in divisions:
                self._filter_division_combo.addItem(d, d)
            self._filter_division_combo.blockSignals(False)
        division = self._filter_division_combo.currentData()
        all_rows = customer_service.list_(division=division)
        self._table.setRowCount(0)
        for r in all_rows:
            self._table.insertRow(self._table.rowCount())
            row = self._table.rowCount() - 1
            self._table.setItem(row, 0, QTableWidgetItem(str(r.get("id", ""))))
            self._table.setItem(row, 1, QTableWidgetItem(r.get("name") or ""))
            self._table.setItem(row, 2, QTableWidgetItem(r.get("division") or ""))
            self._table.setItem(row, 3, QTableWidgetItem(r.get("phone") or ""))

    def _on_add(self):
        name = self._name_edit.text().strip()
        if not name:
            QMessageBox.warning(self, "경고", "고객명을 입력하세요.")
            return
        id_ = customer_service.create({
            "name": name,
            "division": self._division_edit.text().strip(),
            "phone": self._phone_edit.text().strip(),
        })
        if id_:
            QMessageBox.information(self, "알림", "저장되었습니다.")
            self._name_edit.clear()
            self._division_edit.clear()
            self._phone_edit.clear()
            self._divisions_loaded = False
            self.load_list()

    def _on_edit(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 수정할 고객을 선택하세요.")
            return
        ok = customer_service.update(self._selected_id, {
            "name": self._name_edit.text().strip(),
            "division": self._division_edit.text().strip(),
            "phone": self._phone_edit.text().strip(),
        })
        if ok:
            QMessageBox.information(self, "알림", "수정되었습니다.")
            self._divisions_loaded = False
            self.load_list()
        else:
            QMessageBox.warning(self, "경고", "고객명을 입력하세요.")

    def _on_delete(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 삭제할 고객을 선택하세요.")
            return
        if QMessageBox.question(self, "확인", "선택한 고객을 삭제할까요?", QMessageBox.Yes | QMessageBox.No, QMessageBox.No) != QMessageBox.Yes:
            return
        if customer_service.delete(self._selected_id):
            QMessageBox.information(self, "알림", "삭제되었습니다.")
            self._selected_id = None
            self._divisions_loaded = False
            self.load_list()


class ContactTab(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._selected_id = None
        layout = QVBoxLayout(self)
        g = QGroupBox("담당자 목록 (고객별)")
        g_layout = QVBoxLayout(g)
        self._customer_combo = QComboBox()
        self._customer_combo.addItem("— 고객 선택 —", None)
        for c in customer_service.list_():
            self._customer_combo.addItem(c.get("name") or "", c.get("id"))
        self._customer_combo.currentIndexChanged.connect(self._load_contacts)
        h = QHBoxLayout()
        h.addWidget(QLabel("고객:"))
        h.addWidget(self._customer_combo)
        g_layout.addLayout(h)
        self._table = QTableWidget()
        self._table.setColumnCount(5)
        self._table.setHorizontalHeaderLabels(["ID", "담당자명", "직장", "핸드폰", "이메일"])
        self._table.horizontalHeader().setSectionResizeMode(1, QHeaderView.Stretch)
        self._table.setSelectionBehavior(QAbstractItemView.SelectRows)
        self._table.itemSelectionChanged.connect(self._on_select)
        g_layout.addWidget(self._table)
        btn = QHBoxLayout()
        btn.addWidget(QPushButton("추가", clicked=self._on_add))
        btn.addWidget(QPushButton("수정", clicked=self._on_edit))
        btn.addWidget(QPushButton("삭제", clicked=self._on_delete))
        g_layout.addLayout(btn)
        layout.addWidget(g)
        self._form = QGroupBox("담당자 입력/수정")
        f_layout = QGridLayout(self._form)
        f_layout.addWidget(QLabel("담당자명:"), 0, 0)
        self._name_edit = QLineEdit()
        f_layout.addWidget(self._name_edit, 0, 1)
        f_layout.addWidget(QLabel("직장/핸드폰:"), 0, 2)
        self._phone_edit = QLineEdit()
        f_layout.addWidget(self._phone_edit, 0, 3)
        f_layout.addWidget(QLabel("이메일:"), 1, 0)
        self._email_edit = QLineEdit()
        f_layout.addWidget(self._email_edit, 1, 1, 1, 3)
        layout.addWidget(self._form)

    def _on_select(self):
        row = self._table.currentRow()
        self._selected_id = None
        if row >= 0:
            it = self._table.item(row, 0)
            if it:
                try:
                    self._selected_id = int(it.text())
                except ValueError:
                    pass

    def _load_contacts(self):
        self._table.setRowCount(0)
        cid = self._customer_combo.currentData()
        if cid is None:
            return
        for r in contact_service.list_by_customer(cid):
            self._table.insertRow(self._table.rowCount())
            row = self._table.rowCount() - 1
            self._table.setItem(row, 0, QTableWidgetItem(str(r.get("id", ""))))
            self._table.setItem(row, 1, QTableWidgetItem(r.get("name") or ""))
            self._table.setItem(row, 2, QTableWidgetItem(r.get("phone_office") or ""))
            self._table.setItem(row, 3, QTableWidgetItem(r.get("phone_mobile") or ""))
            self._table.setItem(row, 4, QTableWidgetItem(r.get("email") or ""))

    def load_list(self):
        self._customer_combo.clear()
        self._customer_combo.addItem("— 고객 선택 —", None)
        for c in customer_service.list_():
            self._customer_combo.addItem(c.get("name") or "", c.get("id"))
        self._load_contacts()

    def _on_add(self):
        cid = self._customer_combo.currentData()
        if cid is None:
            QMessageBox.warning(self, "경고", "고객을 선택하세요.")
            return
        name = self._name_edit.text().strip()
        if not name:
            QMessageBox.warning(self, "경고", "담당자명을 입력하세요.")
            return
        id_ = contact_service.create({
            "customer_id": cid,
            "name": name,
            "phone_office": self._phone_edit.text().strip(),
            "phone_mobile": "",
            "email": self._email_edit.text().strip(),
        })
        if id_:
            QMessageBox.information(self, "알림", "저장되었습니다.")
            self._name_edit.clear()
            self._phone_edit.clear()
            self._email_edit.clear()
            self._load_contacts()

    def _on_edit(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 수정할 담당자를 선택하세요.")
            return
        ok = contact_service.update(self._selected_id, {
            "customer_id": self._customer_combo.currentData(),
            "name": self._name_edit.text().strip(),
            "phone_office": self._phone_edit.text().strip(),
            "email": self._email_edit.text().strip(),
        })
        if ok:
            QMessageBox.information(self, "알림", "수정되었습니다.")
            self._load_contacts()

    def _on_delete(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 삭제할 담당자를 선택하세요.")
            return
        if QMessageBox.question(self, "확인", "선택한 담당자를 삭제할까요?", QMessageBox.Yes | QMessageBox.No, QMessageBox.No) != QMessageBox.Yes:
            return
        if contact_service.delete(self._selected_id):
            QMessageBox.information(self, "알림", "삭제되었습니다.")
            self._selected_id = None
            self._load_contacts()


class EquipmentTab(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._selected_id = None
        layout = QVBoxLayout(self)
        g = QGroupBox("장비 목록 (고객별)")
        g_layout = QVBoxLayout(g)
        self._customer_combo = QComboBox()
        self._customer_combo.addItem("— 고객 선택 —", None)
        for c in customer_service.list_():
            self._customer_combo.addItem(c.get("name") or "", c.get("id"))
        self._customer_combo.currentIndexChanged.connect(self._load_equipment)
        h = QHBoxLayout()
        h.addWidget(QLabel("고객:"))
        h.addWidget(self._customer_combo)
        g_layout.addLayout(h)
        self._table = QTableWidget()
        self._table.setColumnCount(5)
        self._table.setHorizontalHeaderLabels(["ID", "장비명", "설치위치", "점검주기", "유/무상"])
        self._table.horizontalHeader().setSectionResizeMode(1, QHeaderView.Stretch)
        self._table.setSelectionBehavior(QAbstractItemView.SelectRows)
        self._table.itemSelectionChanged.connect(self._on_select)
        g_layout.addWidget(self._table)
        btn = QHBoxLayout()
        btn.addWidget(QPushButton("추가", clicked=self._on_add))
        btn.addWidget(QPushButton("수정", clicked=self._on_edit))
        btn.addWidget(QPushButton("삭제", clicked=self._on_delete))
        g_layout.addLayout(btn)
        layout.addWidget(g)
        self._form = QGroupBox("장비 입력/수정")
        f_layout = QGridLayout(self._form)
        f_layout.addWidget(QLabel("장비명:"), 0, 0)
        self._name_edit = QLineEdit()
        f_layout.addWidget(self._name_edit, 0, 1)
        f_layout.addWidget(QLabel("설치위치:"), 0, 2)
        self._location_edit = QLineEdit()
        f_layout.addWidget(self._location_edit, 0, 3)
        f_layout.addWidget(QLabel("점검주기/유무상:"), 1, 0)
        self._interval_edit = QLineEdit()
        self._interval_edit.setPlaceholderText("월/Call 등")
        f_layout.addWidget(self._interval_edit, 1, 1)
        self._billing_combo = QComboBox()
        self._billing_combo.addItem("", None)
        self._billing_combo.addItems(config.BILLING_TYPES)
        f_layout.addWidget(self._billing_combo, 1, 2)
        layout.addWidget(self._form)

    def _on_select(self):
        row = self._table.currentRow()
        self._selected_id = None
        if row >= 0:
            it = self._table.item(row, 0)
            if it:
                try:
                    self._selected_id = int(it.text())
                except ValueError:
                    pass

    def _load_equipment(self):
        self._table.setRowCount(0)
        cid = self._customer_combo.currentData()
        if cid is None:
            return
        for r in equipment_service.list_by_customer(cid):
            self._table.insertRow(self._table.rowCount())
            row = self._table.rowCount() - 1
            self._table.setItem(row, 0, QTableWidgetItem(str(r.get("id", ""))))
            self._table.setItem(row, 1, QTableWidgetItem(r.get("name") or ""))
            self._table.setItem(row, 2, QTableWidgetItem(r.get("location") or ""))
            self._table.setItem(row, 3, QTableWidgetItem(r.get("inspection_interval") or ""))
            self._table.setItem(row, 4, QTableWidgetItem(r.get("billing_type") or ""))

    def load_list(self):
        self._customer_combo.clear()
        self._customer_combo.addItem("— 고객 선택 —", None)
        for c in customer_service.list_():
            self._customer_combo.addItem(c.get("name") or "", c.get("id"))
        self._load_equipment()

    def _on_add(self):
        cid = self._customer_combo.currentData()
        if cid is None:
            QMessageBox.warning(self, "경고", "고객을 선택하세요.")
            return
        name = self._name_edit.text().strip()
        if not name:
            QMessageBox.warning(self, "경고", "장비명을 입력하세요.")
            return
        id_ = equipment_service.create({
            "customer_id": cid,
            "name": name,
            "location": self._location_edit.text().strip(),
            "inspection_interval": self._interval_edit.text().strip(),
            "billing_type": self._billing_combo.currentText() or None,
        })
        if id_:
            QMessageBox.information(self, "알림", "저장되었습니다.")
            self._name_edit.clear()
            self._location_edit.clear()
            self._interval_edit.clear()
            self._billing_combo.setCurrentIndex(0)
            self._load_equipment()

    def _on_edit(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 수정할 장비를 선택하세요.")
            return
        ok = equipment_service.update(self._selected_id, {
            "customer_id": self._customer_combo.currentData(),
            "name": self._name_edit.text().strip(),
            "location": self._location_edit.text().strip(),
            "inspection_interval": self._interval_edit.text().strip(),
            "billing_type": self._billing_combo.currentText() or None,
        })
        if ok:
            QMessageBox.information(self, "알림", "수정되었습니다.")
            self._load_equipment()

    def _on_delete(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 삭제할 장비를 선택하세요.")
            return
        if QMessageBox.question(self, "확인", "선택한 장비를 삭제할까요?", QMessageBox.Yes | QMessageBox.No, QMessageBox.No) != QMessageBox.Yes:
            return
        if equipment_service.delete(self._selected_id):
            QMessageBox.information(self, "알림", "삭제되었습니다.")
            self._selected_id = None
            self._load_equipment()


class MasterFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        layout = QVBoxLayout(self)
        tabs = QTabWidget()
        self._customer_tab = CustomerTab()
        self._contact_tab = ContactTab()
        self._equipment_tab = EquipmentTab()
        tabs.addTab(self._customer_tab, "고객")
        tabs.addTab(self._contact_tab, "담당자")
        tabs.addTab(self._equipment_tab, "장비")
        layout.addWidget(tabs)

    def load_list(self):
        self._customer_tab.load_list()
        self._contact_tab.load_list()
        self._equipment_tab.load_list()
