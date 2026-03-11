# -*- coding: utf-8 -*-
"""통합 처리 이력(접수대장) 입력·목록·조회·수정·삭제 — 구현_탭_파일구조_설계.md §3-4."""
from datetime import datetime

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QGridLayout, QGroupBox,
    QLabel, QLineEdit, QComboBox, QTextEdit, QPushButton,
    QTableWidget, QTableWidgetItem, QHeaderView, QMessageBox,
    QAbstractItemView, QFrame,
)
from PySide6.QtCore import Qt

from ... import config
from ...services import work_log_service
from ...services import customer_service
from ...services import contact_service


class WorkLogFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._selected_id = None
        self._build_form()
        self._build_list()

    def _build_form(self):
        form_group = QGroupBox("처리 이력 입력")
        form_layout = QGridLayout(form_group)

        form_layout.addWidget(QLabel("구분:"), 0, 0)
        self._source_combo = QComboBox()
        self._source_combo.addItems(config.SOURCES)
        self._source_combo.setMaximumWidth(100)
        self._source_combo.currentTextChanged.connect(self._on_source_changed)
        form_layout.addWidget(self._source_combo, 0, 1)

        form_layout.addWidget(QLabel("일시:"), 0, 2)
        self._occurred_edit = QLineEdit()
        self._occurred_edit.setPlaceholderText("YYYY-MM-DD 또는 YYYY-MM-DD HH:MM")
        self._occurred_edit.setText(datetime.now().strftime("%Y-%m-%d"))
        self._occurred_edit.setMaximumWidth(180)
        form_layout.addWidget(self._occurred_edit, 0, 3)

        form_layout.addWidget(QLabel("진행:"), 1, 0)
        self._status_combo = QComboBox()
        self._status_combo.addItems(config.STATUS_LIST)
        form_layout.addWidget(self._status_combo, 1, 1)

        form_layout.addWidget(QLabel("접수 방법:"), 1, 2)
        self._reception_combo = QComboBox()
        self._reception_combo.setMaximumWidth(100)
        self._reception_combo.addItem("— 선택 —", None)
        self._reception_combo.addItems(config.RECEPTION_METHODS)
        form_layout.addWidget(self._reception_combo, 1, 3)

        form_layout.addWidget(QLabel("고객:"), 2, 0)
        self._customer_combo = QComboBox()
        self._customer_combo.setMinimumWidth(180)
        self._customer_combo.addItem("— 선택 —", None)
        self._customer_combo.currentIndexChanged.connect(self._on_customer_changed)
        form_layout.addWidget(self._customer_combo, 2, 1, 1, 3)

        form_layout.addWidget(QLabel("내용:"), 3, 0, Qt.AlignTop)
        self._content_edit = QTextEdit()
        self._content_edit.setMaximumHeight(60)
        self._content_edit.setPlaceholderText("내용 (필수)")
        form_layout.addWidget(self._content_edit, 3, 1, 1, 3)

        # NICOM 전용
        self._nicom_widget = QFrame()
        nicom_layout = QGridLayout(self._nicom_widget)
        nicom_layout.addWidget(QLabel("고객담당자:"), 0, 0)
        self._contact_combo = QComboBox()
        self._contact_combo.addItem("— 선택 —", None)
        self._contact_combo.setMinimumWidth(150)
        nicom_layout.addWidget(self._contact_combo, 0, 1)
        nicom_layout.addWidget(QLabel("모델/장비명:"), 0, 2)
        self._equipment_name_edit = QLineEdit()
        nicom_layout.addWidget(self._equipment_name_edit, 0, 3)
        nicom_layout.addWidget(QLabel("장비위치:"), 1, 0)
        self._equipment_location_edit = QLineEdit()
        nicom_layout.addWidget(self._equipment_location_edit, 1, 1)
        nicom_layout.addWidget(QLabel("유/무상:"), 1, 2)
        self._billing_combo = QComboBox()
        self._billing_combo.addItem("", None)
        self._billing_combo.addItems(config.BILLING_TYPES)
        nicom_layout.addWidget(self._billing_combo, 1, 3)
        nicom_layout.addWidget(QLabel("접수내용:"), 2, 0, Qt.AlignTop)
        self._received_edit = QTextEdit()
        self._received_edit.setMaximumHeight(50)
        nicom_layout.addWidget(self._received_edit, 2, 1, 1, 3)
        nicom_layout.addWidget(QLabel("처리내용:"), 3, 0, Qt.AlignTop)
        self._handled_edit = QTextEdit()
        self._handled_edit.setMaximumHeight(50)
        nicom_layout.addWidget(self._handled_edit, 3, 1, 1, 3)
        form_layout.addWidget(self._nicom_widget, 4, 0, 1, 4)

        # 원콜/세종KLAS 전용
        self._oncall_widget = QFrame()
        oncall_layout = QGridLayout(self._oncall_widget)
        oncall_layout.addWidget(QLabel("처리일자:"), 0, 0)
        self._processed_edit = QLineEdit()
        self._processed_edit.setPlaceholderText("YYYY-MM-DD")
        oncall_layout.addWidget(self._processed_edit, 0, 1)
        oncall_layout.addWidget(QLabel("분류:"), 0, 2)
        self._category_combo = QComboBox()
        self._category_combo.addItems(config.CATEGORIES)
        oncall_layout.addWidget(self._category_combo, 0, 3)
        oncall_layout.addWidget(QLabel("제목:"), 1, 0)
        self._title_edit = QLineEdit()
        oncall_layout.addWidget(self._title_edit, 1, 1, 1, 3)
        oncall_layout.addWidget(QLabel("답변:"), 2, 0, Qt.AlignTop)
        self._reply_edit = QTextEdit()
        self._reply_edit.setMaximumHeight(50)
        oncall_layout.addWidget(self._reply_edit, 2, 1, 1, 3)
        form_layout.addWidget(self._oncall_widget, 5, 0, 1, 4)

        form_layout.addWidget(QLabel("비고:"), 6, 0)
        self._remarks_edit = QLineEdit()
        form_layout.addWidget(self._remarks_edit, 6, 1, 1, 3)

        btn_layout = QHBoxLayout()
        btn_layout.addWidget(QPushButton("저장", clicked=self._on_save))
        btn_layout.addWidget(QPushButton("새로 작성", clicked=self._clear_form))
        form_layout.addLayout(btn_layout, 7, 1, 1, 3)

        self._on_source_changed(self._source_combo.currentText())

        layout = QVBoxLayout(self)
        layout.addWidget(form_group)

    def _on_source_changed(self, source: str):
        is_nicom = source == "NICOM"
        self._nicom_widget.setVisible(is_nicom)
        self._oncall_widget.setVisible(not is_nicom)

    def _refresh_customers(self):
        self._customer_combo.clear()
        self._customer_combo.addItem("— 선택 —", None)
        for c in customer_service.list_():
            self._customer_combo.addItem(c.get("name") or "", c.get("id"))

    def _on_customer_changed(self, _index):
        cid = self._customer_combo.currentData()
        self._contact_combo.clear()
        self._contact_combo.addItem("— 선택 —", None)
        if cid is not None:
            for c in contact_service.list_by_customer(cid):
                self._contact_combo.addItem(c.get("name") or "", c.get("id"))

    def _build_list(self):
        list_group = QGroupBox("접수대장·목록")
        list_layout = QVBoxLayout(list_group)

        range_layout = QHBoxLayout()
        range_layout.addWidget(QLabel("구분:"))
        self._filter_source_combo = QComboBox()
        self._filter_source_combo.addItem("전체", None)
        self._filter_source_combo.addItems(config.SOURCES)
        range_layout.addWidget(self._filter_source_combo)
        range_layout.addWidget(QLabel("접수방법:"))
        self._filter_reception_combo = QComboBox()
        self._filter_reception_combo.addItem("전체", None)
        self._filter_reception_combo.addItems(config.RECEPTION_METHODS)
        range_layout.addWidget(self._filter_reception_combo)
        range_layout.addWidget(QLabel("시작일:"))
        self._date_from_edit = QLineEdit()
        self._date_from_edit.setPlaceholderText("YYYY-MM-DD")
        self._date_from_edit.setText(datetime.now().replace(day=1).strftime("%Y-%m-%d"))
        self._date_from_edit.setMaximumWidth(110)
        range_layout.addWidget(self._date_from_edit)
        range_layout.addWidget(QLabel("종료일:"))
        self._date_to_edit = QLineEdit()
        self._date_to_edit.setPlaceholderText("YYYY-MM-DD")
        self._date_to_edit.setText(datetime.now().strftime("%Y-%m-%d"))
        self._date_to_edit.setMaximumWidth(110)
        range_layout.addWidget(self._date_to_edit)
        range_layout.addWidget(QPushButton("조회", clicked=self._on_search))
        range_layout.addStretch()
        list_layout.addLayout(range_layout)

        self._table = QTableWidget()
        self._table.setColumnCount(7)
        self._table.setHorizontalHeaderLabels(["ID", "구분", "일시", "접수방법", "고객", "진행", "내용"])
        self._table.horizontalHeader().setSectionResizeMode(6, QHeaderView.Stretch)
        self._table.setSelectionBehavior(QAbstractItemView.SelectRows)
        self._table.setSelectionMode(QAbstractItemView.SingleSelection)
        self._table.setEditTriggers(QAbstractItemView.NoEditTriggers)
        self._table.itemSelectionChanged.connect(self._on_select)
        list_layout.addWidget(self._table)

        btn_layout = QHBoxLayout()
        btn_layout.addWidget(QPushButton("수정", clicked=self._on_edit))
        btn_layout.addWidget(QPushButton("삭제", clicked=self._on_delete))
        btn_layout.addStretch()
        list_layout.addLayout(btn_layout)

        self.layout().addWidget(list_group)

    def _get_form_data(self):
        cid = self._customer_combo.currentData()
        reception = None
        if self._reception_combo.currentIndex() > 0:
            reception = self._reception_combo.currentText().strip()
        data = {
            "source": self._source_combo.currentText().strip(),
            "occurred_at": self._occurred_edit.text().strip(),
            "status": self._status_combo.currentText().strip(),
            "reception_method": reception,
            "content": self._content_edit.toPlainText().strip(),
            "customer_id": cid,
            "contact_id": self._contact_combo.currentData() if self._source_combo.currentText() == "NICOM" else None,
            "processed_at": self._processed_edit.text().strip() or None,
            "category": self._category_combo.currentText().strip() or None,
            "title": self._title_edit.text().strip() or None,
            "reply": self._reply_edit.toPlainText().strip() or None,
            "equipment_name": self._equipment_name_edit.text().strip() or None,
            "equipment_location": self._equipment_location_edit.text().strip() or None,
            "billing_type": self._billing_combo.currentText().strip() or None,
            "received_content": self._received_edit.toPlainText().strip() or None,
            "handled_content": self._handled_edit.toPlainText().strip() or None,
            "remarks": self._remarks_edit.text().strip() or None,
        }
        return data

    def _set_form_data(self, row: dict):
        self._source_combo.setCurrentText(row.get("source") or "NICOM")
        self._occurred_edit.setText(row.get("occurred_at", "")[:19].replace("T", " "))
        self._status_combo.setCurrentText(row.get("status") or "진행중")
        rec = row.get("reception_method") or ""
        idx = self._reception_combo.findText(rec) if rec else 0
        self._reception_combo.setCurrentIndex(max(0, idx))
        self._content_edit.setPlainText(row.get("content") or "")
        cid = row.get("customer_id")
        idx = self._customer_combo.findData(cid)
        if idx >= 0:
            self._customer_combo.setCurrentIndex(idx)
        self._on_customer_changed(None)
        if row.get("source") == "NICOM":
            self._contact_combo.setCurrentIndex(self._contact_combo.findData(row.get("contact_id")))
            self._equipment_name_edit.setText(row.get("equipment_name") or "")
            self._equipment_location_edit.setText(row.get("equipment_location") or "")
            self._billing_combo.setCurrentText(row.get("billing_type") or "")
            self._received_edit.setPlainText(row.get("received_content") or "")
            self._handled_edit.setPlainText(row.get("handled_content") or "")
        else:
            self._processed_edit.setText((row.get("processed_at") or "")[:10])
            self._category_combo.setCurrentText(row.get("category") or "")
            self._title_edit.setText(row.get("title") or "")
            self._reply_edit.setPlainText(row.get("reply") or "")
        self._remarks_edit.setText(row.get("remarks") or "")
        self._on_source_changed(row.get("source") or "NICOM")

    def _clear_form(self):
        self._selected_id = None
        self._source_combo.setCurrentIndex(0)
        self._occurred_edit.setText(datetime.now().strftime("%Y-%m-%d"))
        self._status_combo.setCurrentIndex(0)
        self._reception_combo.setCurrentIndex(0)
        self._content_edit.clear()
        self._customer_combo.setCurrentIndex(0)
        self._contact_combo.clear()
        self._contact_combo.addItem("— 선택 —", None)
        self._equipment_name_edit.clear()
        self._equipment_location_edit.clear()
        self._billing_combo.setCurrentIndex(0)
        self._received_edit.clear()
        self._handled_edit.clear()
        self._processed_edit.clear()
        self._category_combo.setCurrentIndex(0)
        self._title_edit.clear()
        self._reply_edit.clear()
        self._remarks_edit.clear()
        self._on_source_changed(self._source_combo.currentText())

    def _on_save(self):
        data = self._get_form_data()
        if self._selected_id:
            ok = work_log_service.update(self._selected_id, data)
            if ok:
                QMessageBox.information(self, "알림", "수정되었습니다.")
                self._clear_form()
                self.load_list()
            else:
                QMessageBox.warning(self, "경고", "구분, 일시, 내용, 진행을 모두 입력해 주세요.")
        else:
            id_ = work_log_service.create(data)
            if id_:
                QMessageBox.information(self, "알림", "저장되었습니다.")
                self._clear_form()
                self.load_list()
            else:
                QMessageBox.warning(self, "경고", "구분, 일시, 내용, 진행을 모두 입력해 주세요.")

    def load_list(self):
        self._refresh_customers()
        self._on_search()

    def _on_search(self):
        self._table.setRowCount(0)
        date_from = self._date_from_edit.text().strip() or None
        date_to = self._date_to_edit.text().strip() or None
        source = None
        if self._filter_source_combo.currentIndex() > 0:
            source = self._filter_source_combo.currentText()
        reception_method = None
        if self._filter_reception_combo.currentIndex() > 0:
            reception_method = self._filter_reception_combo.currentText()
        rows = work_log_service.list_(
            source=source,
            date_from=date_from,
            date_to=date_to,
            reception_method=reception_method,
        )
        customers = {c["id"]: c.get("name") for c in customer_service.list_()}
        for r in rows:
            content = (r.get("content") or "")[:35]
            if len((r.get("content") or "")) > 35:
                content += "…"
            cust_name = customers.get(r.get("customer_id"), "") if r.get("customer_id") else ""
            row_pos = self._table.rowCount()
            self._table.insertRow(row_pos)
            self._table.setItem(row_pos, 0, QTableWidgetItem(str(r.get("id", ""))))
            self._table.setItem(row_pos, 1, QTableWidgetItem(r.get("source") or ""))
            self._table.setItem(row_pos, 2, QTableWidgetItem((r.get("occurred_at") or "")[:16].replace("T", " ")))
            self._table.setItem(row_pos, 3, QTableWidgetItem(r.get("reception_method") or ""))
            self._table.setItem(row_pos, 4, QTableWidgetItem(cust_name))
            self._table.setItem(row_pos, 5, QTableWidgetItem(r.get("status") or ""))
            self._table.setItem(row_pos, 6, QTableWidgetItem(content))

    def _on_select(self):
        row = self._table.currentRow()
        if row < 0:
            self._selected_id = None
            return
        id_item = self._table.item(row, 0)
        if id_item:
            try:
                self._selected_id = int(id_item.text())
            except ValueError:
                self._selected_id = None

    def _on_edit(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 수정할 항목을 선택하세요.")
            return
        row = work_log_service.get(self._selected_id)
        if row:
            self._refresh_customers()
            self._set_form_data(row)

    def _on_delete(self):
        if self._selected_id is None:
            QMessageBox.information(self, "알림", "목록에서 삭제할 항목을 선택하세요.")
            return
        reply = QMessageBox.question(
            self, "확인", "선택한 처리 이력을 삭제할까요?",
            QMessageBox.Yes | QMessageBox.No, QMessageBox.No
        )
        if reply != QMessageBox.Yes:
            return
        if work_log_service.delete(self._selected_id):
            QMessageBox.information(self, "알림", "삭제되었습니다.")
            self._selected_id = None
            self._clear_form()
            self.load_list()
        else:
            QMessageBox.critical(self, "오류", "삭제에 실패했습니다.")
