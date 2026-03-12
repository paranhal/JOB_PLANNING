# SW상세관리 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QFormLayout, QLabel, QLineEdit,
    QListWidget, QGroupBox, QPushButton, QMessageBox, QComboBox,
    QSplitter, QScrollArea,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service, installation_service
from as_support.ui.widgets.code_combo import CodeCombo


class InstallationSwFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._customer_id = None
        self._installation_id = None
        self._current_sw_id = None
        self._build_ui()
        self._refresh_customers()

    def _build_ui(self):
        layout = QVBoxLayout(self)
        top = QHBoxLayout()
        top.addWidget(QLabel("기관:"))
        self._customer_combo = QComboBox()
        self._customer_combo.setMinimumWidth(200)
        self._customer_combo.currentIndexChanged.connect(self._on_customer_change)
        top.addWidget(self._customer_combo)
        top.addWidget(QLabel("설치자산:"))
        self._inst_combo = QComboBox()
        self._inst_combo.setMinimumWidth(220)
        self._inst_combo.currentIndexChanged.connect(self._on_inst_change)
        top.addWidget(self._inst_combo)
        layout.addLayout(top)

        split = QSplitter(Qt.Orientation.Horizontal)
        list_gb = QGroupBox("SW 상세 목록")
        list_layout = QVBoxLayout(list_gb)
        self._list = QListWidget()
        self._list.currentItemChanged.connect(self._on_select)
        list_layout.addWidget(self._list)
        split.addWidget(list_gb)

        detail_gb = QGroupBox("SW 상세 정보")
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
        self._sw_name = QLineEdit()
        self._sw_ver = QLineEdit()
        self._os_name = QLineEdit()
        self._os_ver = QLineEdit()
        self._access_type = QLineEdit()
        self._access_addr = QLineEdit()
        form.addRow("소프트웨어명:", self._sw_name)
        form.addRow("버전:", self._sw_ver)
        form.addRow("OS:", self._os_name)
        form.addRow("OS버전:", self._os_ver)
        form.addRow("접속방식:", self._access_type)
        form.addRow("접속주소:", self._access_addr)
        scroll.setWidget(form_widget)
        detail_layout.addWidget(scroll)
        split.addWidget(detail_gb)
        split.setSizes([280, 400])
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
        self._inst_combo.clear()
        if not self._customer_id:
            self._installation_id = None
            self._refresh_list()
            return
        inst_list = installation_service.list_by_customer(self._customer_id)
        for i in inst_list:
            label = (i.get("product_name") or "") + " " + (i.get("serial_number") or "")
            self._inst_combo.addItem(label.strip(), i.get("installation_id"))
        if self._inst_combo.count():
            self._inst_combo.setCurrentIndex(0)
        self._on_inst_change()

    def _on_inst_change(self):
        self._installation_id = self._inst_combo.currentData()
        self._refresh_list()

    def _refresh_list(self):
        self._list.clear()
        if not self._installation_id:
            return
        for sw in installation_service.list_sw(self._installation_id):
            self._list.addItem((sw.get("software_name") or "") + " " + (sw.get("software_version") or ""))

    def _on_select(self, current, previous):
        if not current or not self._installation_id:
            return
        idx = self._list.currentRow()
        sw_list = installation_service.list_sw(self._installation_id)
        if idx >= len(sw_list):
            return
        sw = sw_list[idx]
        self._current_sw_id = sw.get("sw_detail_id")
        self._sw_name.setText(sw.get("software_name") or "")
        self._sw_ver.setText(sw.get("software_version") or "")
        self._os_name.setText(sw.get("os_name") or "")
        self._os_ver.setText(sw.get("os_version") or "")
        self._access_type.setText(sw.get("access_type") or "")
        self._access_addr.setText(sw.get("access_address") or "")

    def _new(self):
        self._current_sw_id = None
        self._sw_name.clear()
        self._sw_ver.clear()
        self._os_name.clear()
        self._os_ver.clear()
        self._access_type.clear()
        self._access_addr.clear()

    def _save(self):
        if not self._installation_id:
            QMessageBox.warning(self, "선택", "설치자산을 선택하세요.")
            return
        data = {
            "installation_id": self._installation_id,
            "software_name": self._sw_name.text().strip(),
            "software_version": self._sw_ver.text().strip(),
            "os_name": self._os_name.text().strip(),
            "os_version": self._os_ver.text().strip(),
            "access_type": self._access_type.text().strip(),
            "access_address": self._access_addr.text().strip(),
        }
        try:
            if self._current_sw_id:
                installation_service.update_sw(self._current_sw_id, data)
                QMessageBox.information(self, "저장", "수정되었습니다.")
            else:
                installation_service.add_sw(data)
                QMessageBox.information(self, "저장", "등록되었습니다.")
            self._refresh_list()
            self._new()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))
