# 설치자산관리 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QFormLayout, QLabel, QLineEdit,
    QListWidget, QGroupBox, QPushButton, QMessageBox, QCheckBox, QComboBox,
    QSplitter, QScrollArea,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service, installation_service
from as_support.ui.widgets.code_combo import CodeCombo
from as_support.ui.widgets.photo_slot import PhotoSlotWidget


class InstallationFrame(QWidget):
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
        list_gb = QGroupBox("설치자산 목록")
        list_layout = QVBoxLayout(list_gb)
        self._list = QListWidget()
        self._list.currentItemChanged.connect(self._on_select)
        list_layout.addWidget(self._list)
        split.addWidget(list_gb)

        detail_gb = QGroupBox("설치 정보")
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
        self._product_name = QLineEdit()
        self._product_type = CodeCombo(self, "product_type")
        self._serial = QLineEdit()
        self._install_owner = CodeCombo(self, "install_owner")
        self._mgmt_type = CodeCombo(self, "management_type")
        self._op_status = CodeCombo(self, "operation_status")
        self._managed_us = QCheckBox("당사 관리대상")
        self._managed_us.setChecked(True)
        self._location_detail = QLineEdit()
        form.addRow("제품명:", self._product_name)
        form.addRow("제품구분:", self._product_type)
        form.addRow("S/N:", self._serial)
        form.addRow("설치주체:", self._install_owner)
        form.addRow("관리유형:", self._mgmt_type)
        form.addRow("운영상태:", self._op_status)
        form.addRow(self._managed_us)
        form.addRow("상세위치:", self._location_detail)
        self._photo1 = PhotoSlotWidget(self, "사진 1")
        self._photo2 = PhotoSlotWidget(self, "사진 2")
        form.addRow("사진 1:", self._photo1)
        form.addRow("사진 2:", self._photo2)
        scroll.setWidget(form_widget)
        detail_layout.addWidget(scroll)
        split.addWidget(detail_gb)
        split.setSizes([300, 450])
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
        for inst in installation_service.list_by_customer(self._customer_id):
            label = (inst.get("product_name") or "(제품명 없음)") + " " + (inst.get("serial_number") or "")
            self._list.addItem(label.strip())
        for w in (self._product_type, self._install_owner, self._mgmt_type, self._op_status):
            w.refresh()

    def _on_select(self, current, previous):
        if not current or not self._customer_id:
            return
        idx = self._list.currentRow()
        items = installation_service.list_by_customer(self._customer_id)
        if idx >= len(items):
            return
        inst = items[idx]
        self._current_id = inst.get("installation_id")
        self._product_name.setText(inst.get("product_name") or "")
        self._product_type.set_code_value(inst.get("product_type_code"))
        self._serial.setText(inst.get("serial_number") or "")
        self._install_owner.set_code_value(inst.get("install_owner_code"))
        self._mgmt_type.set_code_value(inst.get("management_type_code"))
        self._op_status.set_code_value(inst.get("operation_status_code"))
        self._managed_us.setChecked(inst.get("managed_by_us_yn", True))
        self._location_detail.setText(inst.get("location_detail") or "")
        urls = inst.get("photo_urls") or [""] * 2
        self._photo1.set_url(urls[0] if len(urls) > 0 else "")
        self._photo2.set_url(urls[1] if len(urls) > 1 else "")

    def _new(self):
        self._current_id = None
        self._product_name.clear()
        self._product_type.set_code_value(None)
        self._serial.clear()
        self._install_owner.set_code_value(None)
        self._mgmt_type.set_code_value(None)
        self._op_status.set_code_value(None)
        self._managed_us.setChecked(True)
        self._location_detail.clear()
        self._photo1.set_url("")
        self._photo2.set_url("")

    def _save(self):
        if not self._customer_id:
            QMessageBox.warning(self, "선택", "기관을 선택하세요.")
            return
        data = {
            "customer_id": self._customer_id,
            "product_name": self._product_name.text().strip(),
            "product_type_code": self._product_type.get_code_value(),
            "serial_number": self._serial.text().strip(),
            "install_owner_code": self._install_owner.get_code_value(),
            "management_type_code": self._mgmt_type.get_code_value(),
            "operation_status_code": self._op_status.get_code_value(),
            "managed_by_us_yn": self._managed_us.isChecked(),
            "location_detail": self._location_detail.text().strip(),
            "photo_urls": [self._photo1.get_url(), self._photo2.get_url()],
        }
        try:
            if self._current_id:
                installation_service.update(self._current_id, data)
                QMessageBox.information(self, "저장", "수정되었습니다.")
            else:
                installation_service.add(data)
                QMessageBox.information(self, "저장", "등록되었습니다.")
            self._refresh_list()
            self._new()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))
