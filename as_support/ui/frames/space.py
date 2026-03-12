# 공간관리 — 건물/층/실 (PySide6)

from PySide6.QtWidgets import (
    QWidget, QVBoxLayout, QHBoxLayout, QFormLayout, QLabel, QLineEdit,
    QTreeWidget, QTreeWidgetItem, QGroupBox, QPushButton, QMessageBox,
    QInputDialog, QComboBox,
)
from PySide6.QtCore import Qt

from as_support.services import customer_service, space_service
from as_support.ui.widgets.code_combo import CodeCombo


class SpaceFrame(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self._customer_id = None
        self._current_type = None
        self._current_id = None
        self._name_edit = None
        self._type_combo = None
        self._room_use = None
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

        tree_gb = QGroupBox("건물 / 층 / 실")
        tree_layout = QVBoxLayout(tree_gb)
        self._tree = QTreeWidget()
        self._tree.setHeaderLabel("구분")
        self._tree.currentItemChanged.connect(self._on_tree_select)
        tree_layout.addWidget(self._tree)
        btn_row = QHBoxLayout()
        btn_row.addWidget(QPushButton("건물 추가", clicked=self._add_building))
        btn_row.addWidget(QPushButton("층 추가", clicked=self._add_floor))
        btn_row.addWidget(QPushButton("실 추가", clicked=self._add_room))
        tree_layout.addLayout(btn_row)
        layout.addWidget(tree_gb)

        self._detail_gb = QGroupBox("상세")
        self._detail_layout = QVBoxLayout(self._detail_gb)
        layout.addWidget(self._detail_gb)

    def _refresh_customers(self):
        self._customer_combo.clear()
        for c in customer_service.list_all():
            self._customer_combo.addItem(c.get("name") or "", c.get("customer_id"))
        if self._customer_combo.count():
            self._customer_combo.setCurrentIndex(0)
            self._on_customer_change()

    def _on_customer_change(self):
        cid = self._customer_combo.currentData()
        self._customer_id = cid
        self._refresh_tree()

    def _refresh_tree(self):
        self._tree.clear()
        if not self._customer_id:
            return
        for b in space_service.list_buildings(self._customer_id):
            bid = b.get("building_id")
            name = b.get("building_name") or "(건물)"
            bitem = QTreeWidgetItem(self._tree, [name])
            bitem.setData(0, Qt.ItemDataRole.UserRole, ("building", bid))
            for fl in space_service.list_floors(bid):
                fid = fl.get("floor_id")
                fname = fl.get("floor_name") or "(층)"
                fitem = QTreeWidgetItem(bitem, [fname])
                fitem.setData(0, Qt.ItemDataRole.UserRole, ("floor", fid))
                for r in space_service.list_rooms(fid):
                    rid = r.get("room_id")
                    rname = r.get("room_name") or "(실)"
                    ritem = QTreeWidgetItem(fitem, [rname])
                    ritem.setData(0, Qt.ItemDataRole.UserRole, ("room", rid))
        self._tree.expandAll()

    def _on_tree_select(self, current, previous):
        self._clear_detail()
        if not current:
            return
        data = current.data(0, Qt.ItemDataRole.UserRole)
        if not data:
            return
        typ, iid = data
        self._current_type = typ
        self._current_id = iid
        if typ == "building":
            b = space_service.get_building(iid)
            self._name_edit = QLineEdit(b.get("building_name") or "" if b else "")
            self._type_combo = CodeCombo(self, "building_type")
            if b:
                self._type_combo.set_code_value(b.get("building_type_code"))
            form = QFormLayout()
            form.addRow("건물명:", self._name_edit)
            form.addRow("건물구분:", self._type_combo)
            save_btn = QPushButton("저장", clicked=self._save_building)
            form.addRow(save_btn)
            self._detail_layout.addLayout(form)
        elif typ == "floor":
            parent_id = current.parent().data(0, Qt.ItemDataRole.UserRole)[1]
            fl_list = space_service.list_floors(parent_id)
            fl = next((x for x in fl_list if x.get("floor_id") == iid), None)
            self._name_edit = QLineEdit(fl.get("floor_name") or "" if fl else "")
            form = QFormLayout()
            form.addRow("층명:", self._name_edit)
            form.addRow(QPushButton("저장", clicked=self._save_floor))
            self._detail_layout.addLayout(form)
        elif typ == "room":
            fid = current.parent().data(0, Qt.ItemDataRole.UserRole)[1]
            r_list = space_service.list_rooms(fid)
            r = next((x for x in r_list if x.get("room_id") == iid), None)
            self._name_edit = QLineEdit(r.get("room_name") or "" if r else "")
            self._room_use = CodeCombo(self, "room_use")
            if r:
                self._room_use.set_code_value(r.get("room_use_code"))
            form = QFormLayout()
            form.addRow("실명:", self._name_edit)
            form.addRow("용도:", self._room_use)
            form.addRow(QPushButton("저장", clicked=self._save_room))
            self._detail_layout.addLayout(form)

    def _clear_detail(self):
        while self._detail_layout.count():
            item = self._detail_layout.takeAt(0)
            if item.widget():
                item.widget().deleteLater()
            elif item.layout():
                self._clear_layout(item.layout())
        self._name_edit = self._type_combo = self._room_use = None

    def _clear_layout(self, lay):
        while lay.count():
            item = lay.takeAt(0)
            if item.widget():
                item.widget().deleteLater()
            elif item.layout():
                self._clear_layout(item.layout())

    def _add_building(self):
        if not self._customer_id:
            QMessageBox.warning(self, "선택", "기관을 선택하세요.")
            return
        name, ok = QInputDialog.getText(self, "건물 추가", "건물명:")
        if not ok or not name.strip():
            return
        try:
            space_service.add_building({"customer_id": self._customer_id, "building_name": name.strip()})
            QMessageBox.information(self, "완료", "건물이 추가되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _add_floor(self):
        current = self._tree.currentItem()
        if not current:
            QMessageBox.warning(self, "선택", "건물을 선택하세요.")
            return
        data = current.data(0, Qt.ItemDataRole.UserRole)
        if not data or data[0] != "building":
            QMessageBox.warning(self, "선택", "건물을 선택한 뒤 층을 추가하세요.")
            return
        name, ok = QInputDialog.getText(self, "층 추가", "층명 (예: 1F, B1):")
        if not ok or not name.strip():
            return
        try:
            space_service.add_floor({"building_id": data[1], "floor_name": name.strip()})
            QMessageBox.information(self, "완료", "층이 추가되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _add_room(self):
        current = self._tree.currentItem()
        if not current:
            QMessageBox.warning(self, "선택", "층을 선택하세요.")
            return
        data = current.data(0, Qt.ItemDataRole.UserRole)
        if not data or data[0] != "floor":
            QMessageBox.warning(self, "선택", "층을 선택한 뒤 실을 추가하세요.")
            return
        name, ok = QInputDialog.getText(self, "실 추가", "실명:")
        if not ok or not name.strip():
            return
        try:
            space_service.add_room({"floor_id": data[1], "room_name": name.strip()})
            QMessageBox.information(self, "완료", "실이 추가되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _save_building(self):
        if not self._current_id or not self._name_edit:
            return
        try:
            space_service.update_building(self._current_id, {
                "building_name": self._name_edit.text().strip(),
                "building_type_code": self._type_combo.get_code_value() if self._type_combo else None,
            })
            QMessageBox.information(self, "저장", "저장되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _save_floor(self):
        if not self._current_id or not self._name_edit:
            return
        try:
            space_service.update_floor(self._current_id, {"floor_name": self._name_edit.text().strip()})
            QMessageBox.information(self, "저장", "저장되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))

    def _save_room(self):
        if not self._current_id or not self._name_edit:
            return
        try:
            space_service.update_room(self._current_id, {
                "room_name": self._name_edit.text().strip(),
                "room_use_code": self._room_use.get_code_value() if self._room_use else None,
            })
            QMessageBox.information(self, "저장", "저장되었습니다.")
            self._refresh_tree()
        except Exception as e:
            QMessageBox.critical(self, "오류", str(e))
