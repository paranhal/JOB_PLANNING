# 담당자관리 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QFormLayout, QLabel, QLineEdit,
    QListWidget, QGroupBox, QPushButton, QMessageBox, QCheckBox, QComboBox,
    QSplitter, QScrollArea,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service, contact_service
from as_support.ui.widgets.code_combo import CodeCombo


class ContactFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._customer_id = None
        self._current_id = None
        self._build_ui()
        self._refresh_customers()

    def _build_ui(self):
        layout = QVBoxLayout(self)
        top = QHBoxLayout()
        top.addWidget(QLabel("기관:"))
        self._customer_combo = QComboBox()
        self._customer_combo.setMinimumWidth(250)
        self._customer_combo.currentIndexChanged.connect(self._on_customer_change)
        top.addWidget(self._customer_combo)
        layout.addLayout(top)

        split = QSplitter(Qt.Orientation.Horizontal)
        list_gb = QGroupBox("담당자 목록")
        list_layout = QVBoxLayout(list_gb)
        self._list = QListWidget()
        self._list.currentItemChanged.connect(self._on_select)
        list_layout.addWidget(self._list)
        split.addWidget(list_gb)

        detail_gb = QGroupBox("담당자 정보")
        detail_layout = QVBoxLayout(detail_gb)
        btn_row = QHBoxLayout()
        btn_row.addWidget(QPushButton("신규", clicked=self._new))
        btn_row.addWidget(QPushButton("저장", clicked=self._save))
        detail_layout.addLayout(btn_row)
        scroll = QScrollArea()
        scroll.setWidgetResizable(True)
        scroll.setHorizontalScrollBarPolicy(Qt.ScrollBarPolicy.ScrollBarAsNeeded)
        scroll.setVerticalScrollBarPolicy(Qt.ScrollBarPolicy.ScrollBarAsNeeded)
        form_widget = QWidget()
        form = QFormLayout(form_widget)
        self._name = QLineEdit()
        self._duty = CodeCombo(self, "duty")
        self._phone = QLineEdit()
        self._mobile = QLineEdit()
        self._email = QLineEdit()
        self._main_yn = QCheckBox("주담당")
        self._in_office_yn = QCheckBox("재직")
        self._in_office_yn.setChecked(True)
        form.addRow("이름:", self._name)
        form.addRow("담당업무:", self._duty)
        form.addRow("전화:", self._phone)
        form.addRow("핸드폰:", self._mobile)
        form.addRow("이메일:", self._email)
        form.addRow(self._main_yn)
        form.addRow(self._in_office_yn)
        scroll.setWidget(form_widget)
        detail_layout.addWidget(scroll)
        split.addWidget(detail_gb)
        split.setSizes([250, 400])
        layout.addWidget(split)

    def _refresh_customers(self):
        self._customer_combo.clear()
        for c in customer_service.list_all():
            self._customer_combo.addItem(c.get("name") or "", c.get("customer_id"))
        if self._customer_combo.count():
            self._customer_combo.setCurrentIndex(0)
            self._on_customer_change()

    def _on_customer_change(self):
        self._customer_id = self._customer_combo.currentData()
        self._refresh_list()

    def _refresh_list(self):
        self._list.clear()
        if not self._customer_id:
            return
        for ct in contact_service.list_by_customer(self._customer_id):
            self._list.addItem(ct.get("name") or "(이름 없음)")
        self._duty.refresh()

    def _on_select(self, current, previous):
        if not current or not self._customer_id:
            return
        name = current.text()
        for ct in contact_service.list_by_customer(self._customer_id, in_office_only=False):
            if (ct.get("name") or "") == name:
                self._current_id = ct.get("contact_id")
                self._name.setText(ct.get("name") or "")
                self._duty.set_code_value(ct.get("duty_code"))
                self._phone.setText(ct.get("phone") or "")
                self._mobile.setText(ct.get("mobile") or "")
                self._email.setText(ct.get("email") or "")
                self._main_yn.setChecked(ct.get("main_contact_yn", False))
                self._in_office_yn.setChecked(ct.get("in_office_yn", True))
                return

    def _new(self):
        self._current_id = None
        self._name.clear()
        self._duty.set_code_value(None)
        self._phone.clear()
        self._mobile.clear()
        self._email.clear()
        self._main_yn.setChecked(False)
        self._in_office_yn.setChecked(True)

    def _save(self):
        if not self._customer_id:
            QMessageBox.warning(self, "선택", "기관을 선택하세요.")
            return
        name = self._name.text().strip()
        if not name:
            QMessageBox.warning(self, "입력", "이름을 입력하세요.")
            return
        data = {
            "customer_id": self._customer_id,
            "name": name,
            "duty_code": self._duty.get_code_value(),
            "phone": self._phone.text().strip(),
            "mobile": self._mobile.text().strip(),
            "email": self._email.text().strip(),
            "main_contact_yn": self._main_yn.isChecked(),
            "in_office_yn": self._in_office_yn.isChecked(),
        }
        try:
            if self._current_id:
                contact_service.update(self._current_id, data)
                QMessageBox.information(self, "저장", "수정되었습니다.")
            else:
                contact_service.add(data)
                QMessageBox.information(self, "저장", "등록되었습니다.")
            self._refresh_list()
            self._new()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))
