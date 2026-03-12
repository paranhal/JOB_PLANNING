# 고객관리 화면 — 기획서 §13 기준정보 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QFormLayout, QLabel, QLineEdit,
    QListWidget, QListWidgetItem, QGroupBox, QPushButton, QMessageBox,
    QSplitter, QFrame, QCheckBox, QComboBox, QScrollArea,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service
from as_support.ui.widgets.code_combo import CodeCombo
from as_support.ui.widgets.photo_slot import PhotoSlotWidget


class CustomerFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._current_id = None
        self._build_ui()
        self.refresh_list()

    def _build_ui(self):
        layout = QVBoxLayout(self)
        # 검색
        search_row = QHBoxLayout()
        search_row.addWidget(QLabel("검색:"))
        self._search = QLineEdit()
        self._search.setPlaceholderText("기관명·공식명칭")
        self._search.textChanged.connect(self.refresh_list)
        search_row.addWidget(self._search)
        layout.addLayout(search_row)

        split = QSplitter(Qt.Orientation.Horizontal)
        # 목록
        list_gb = QGroupBox("기관 목록")
        list_layout = QVBoxLayout(list_gb)
        self._list = QListWidget()
        self._list.currentItemChanged.connect(self._on_select)
        list_layout.addWidget(self._list)
        split.addWidget(list_gb)

        # 상세 폼 — 버튼 상단, 폼은 스크롤 영역
        detail_gb = QGroupBox("기관 정보")
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
        form.setLabelAlignment(Qt.AlignmentFlag.AlignLeft)
        form.setFormAlignment(Qt.AlignmentFlag.AlignLeft | Qt.AlignmentFlag.AlignTop)
        field_min_width = 380
        self._name = QLineEdit()
        self._name.setMinimumWidth(field_min_width)
        self._official_name = QLineEdit()
        self._official_name.setMinimumWidth(field_min_width)
        self._phone = QLineEdit()
        self._phone.setMinimumWidth(field_min_width)
        self._industry = CodeCombo(self, "industry")
        self._industry.setMinimumWidth(field_min_width)
        self._parent_yn = QCheckBox("상위기관 있음")
        self._parent_yn.toggled.connect(lambda on: self._parent_combo.setEnabled(on))
        self._parent_combo = QComboBox()
        self._parent_combo.setMinimumWidth(field_min_width)
        self._parent_combo.setEditable(False)
        self._parent_combo.setEnabled(False)
        self._email = QLineEdit()
        self._email.setMinimumWidth(field_min_width)
        self._business_number = QLineEdit()
        self._business_number.setMinimumWidth(field_min_width)
        self._address = QLineEdit()
        self._address.setMinimumWidth(field_min_width)
        self._address.setMinimumHeight(28)
        self._use_yn = QCheckBox("사용")
        self._use_yn.setChecked(True)
        self._remarks = QLineEdit()
        self._remarks.setMinimumWidth(field_min_width)
        self._remarks.setMinimumHeight(28)
        form.addRow("기관명:", self._name)
        form.addRow("공식명칭:", self._official_name)
        form.addRow("대표전화:", self._phone)
        form.addRow("업종:", self._industry)
        form.addRow("상위기관 여부:", self._parent_yn)
        form.addRow("상위기관:", self._parent_combo)
        form.addRow("이메일:", self._email)
        form.addRow("사업자번호:", self._business_number)
        form.addRow("주소:", self._address)
        form.addRow(self._use_yn)
        form.addRow("비고:", self._remarks)
        form.addRow(QLabel("사진 (링크만 저장, 미리보기 표시):"))
        self._photo1 = PhotoSlotWidget(self, "사진 1")
        self._photo2 = PhotoSlotWidget(self, "사진 2")
        self._photo3 = PhotoSlotWidget(self, "사진 3")
        self._photo4 = PhotoSlotWidget(self, "사진 4")
        form.addRow("사진 1:", self._photo1)
        form.addRow("사진 2:", self._photo2)
        form.addRow("사진 3:", self._photo3)
        form.addRow("사진 4:", self._photo4)
        scroll.setWidget(form_widget)
        detail_layout.addWidget(scroll)
        split.addWidget(detail_gb)
        split.setSizes([280, 520])
        layout.addWidget(split)

    def _refresh_parent_combo(self):
        """상위기관 콤보: (없음) + 현재 기관 제외한 기관 목록"""
        self._parent_combo.clear()
        self._parent_combo.addItem("(없음)", None)
        for c in customer_service.list_all():
            if c.get("customer_id") == self._current_id:
                continue
            name = c.get("name") or "(이름 없음)"
            self._parent_combo.addItem(name, c.get("customer_id"))

    def _get_form_data(self):
        parent_id = None
        if self._parent_yn.isChecked():
            parent_id = self._parent_combo.currentData()
        return {
            "name": self._name.text().strip(),
            "official_name": self._official_name.text().strip(),
            "phone": self._phone.text().strip(),
            "industry_code": self._industry.get_code_value() or "",
            "parent_customer_id": parent_id,
            "email": self._email.text().strip(),
            "business_number": self._business_number.text().strip(),
            "address": self._address.text().strip(),
            "use_yn": self._use_yn.isChecked(),
            "remarks": self._remarks.text().strip(),
            "photo_urls": [
                self._photo1.get_url(),
                self._photo2.get_url(),
                self._photo3.get_url(),
                self._photo4.get_url(),
            ],
        }

    def _set_form(self, c: dict | None):
        self._current_id = None
        self._name.clear()
        self._official_name.clear()
        self._phone.clear()
        self._industry.set_code_value(None)
        self._parent_yn.setChecked(False)
        self._parent_combo.setCurrentIndex(0)
        self._email.clear()
        self._business_number.clear()
        self._address.clear()
        self._use_yn.setChecked(True)
        self._remarks.clear()
        for slot in (self._photo1, self._photo2, self._photo3, self._photo4):
            slot.set_url("")
        if not c:
            return
        self._current_id = c.get("customer_id")
        self._name.setText(c.get("name") or "")
        self._official_name.setText(c.get("official_name") or "")
        self._phone.setText(c.get("phone") or "")
        self._industry.set_code_value(c.get("industry_code"))
        self._refresh_parent_combo()
        parent_id = c.get("parent_customer_id")
        if parent_id:
            self._parent_yn.setChecked(True)
            for i in range(self._parent_combo.count()):
                if self._parent_combo.itemData(i) == parent_id:
                    self._parent_combo.setCurrentIndex(i)
                    break
        else:
            self._parent_yn.setChecked(False)
            self._parent_combo.setCurrentIndex(0)
        self._email.setText(c.get("email") or "")
        self._business_number.setText(c.get("business_number") or "")
        self._address.setText(c.get("address") or "")
        self._use_yn.setChecked(c.get("use_yn", True))
        self._remarks.setText(c.get("remarks") or "")
        urls = c.get("photo_urls") or [""] * 4
        for i, slot in enumerate((self._photo1, self._photo2, self._photo3, self._photo4)):
            slot.set_url(urls[i] if i < len(urls) else "")

    def _on_select(self, current, previous):
        if not current:
            return
        name = current.text()
        for c in customer_service.list_all():
            if (c.get("name") or "") == name:
                self._set_form(c)
                return

    def refresh_list(self):
        kw = self._search.text().strip()
        items = customer_service.search(kw) if kw else customer_service.list_all()
        self._list.clear()
        for c in items:
            self._list.addItem(c.get("name") or "(이름 없음)")
        self._industry.refresh()
        self._refresh_parent_combo()

    def _new(self):
        self._set_form(None)

    def _save(self):
        data = self._get_form_data()
        if not data.get("name"):
            QMessageBox.warning(self, "입력", "기관명을 입력하세요.")
            return
        if not data.get("phone"):
            QMessageBox.warning(self, "입력", "대표전화를 입력하세요.")
            return
        try:
            if self._current_id:
                customer_service.update(self._current_id, data)
                QMessageBox.information(self, "저장", "수정되었습니다.")
            else:
                customer_service.add(data)
                QMessageBox.information(self, "저장", "등록되었습니다.")
            self.refresh_list()
            self._set_form(None)
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))
