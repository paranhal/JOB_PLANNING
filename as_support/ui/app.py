# 메인 윈도우 — 기획서 §13 메뉴·탭 (1단계) — PySide6

from PySide6.QtWidgets import (
    QApplication, QMainWindow, QWidget, QVBoxLayout, QHBoxLayout,
    QTabWidget, QLabel, QPushButton, QSplitter, QFrame, QScrollArea,
)
from PySide6.QtCore import Qt

from as_support.services import code_service
from as_support.ui.frames import (
    CustomerFrame,
    SpaceFrame,
    ContactFrame,
    ContactHistoryFrame,
    InstallationFrame,
    InstallationSwFrame,
    CodeMasterFrame,
)


class App(QMainWindow):
    def __init__(self):
        super().__init__()
        code_service.ensure_defaults()
        self.setWindowTitle("고객지원 시스템 (1단계)")
        self.resize(1000, 650)
        self._build_ui()

    def _build_ui(self):
        central = QWidget()
        self.setCentralWidget(central)
        layout = QHBoxLayout(central)

        # 좌측 메뉴
        menu_frame = QFrame()
        menu_frame.setFrameStyle(QFrame.Shape.StyledPanel)
        menu_layout = QVBoxLayout(menu_frame)
        menu_layout.addWidget(QLabel("메뉴"))
        for i, (label, key) in enumerate([("기준정보", 0), ("자산관리", 1), ("관리", 2)]):
            btn = QPushButton(label)
            btn.setFixedWidth(100)
            btn.clicked.connect(lambda checked, k=key: self._tabs.setCurrentIndex(k))
            menu_layout.addWidget(btn)
        menu_layout.addStretch()
        layout.addWidget(menu_frame)

        # 중앙 탭
        self._tabs = QTabWidget()
        self._tabs.addTab(self._make_base_tab(), "기준정보")
        self._tabs.addTab(self._make_asset_tab(), "자산관리")
        self._tabs.addTab(self._make_mgmt_tab(), "관리")
        layout.addWidget(self._tabs, stretch=1)

    def _make_base_tab(self):
        w = QWidget()
        sub = QTabWidget()
        sub.addTab(CustomerFrame(), "고객관리")
        sub.addTab(SpaceFrame(), "공간관리")
        sub.addTab(ContactFrame(), "담당자관리")
        sub.addTab(ContactHistoryFrame(), "담당자이력")
        lay = QVBoxLayout(w)
        lay.setContentsMargins(0, 0, 0, 0)
        lay.addWidget(sub)
        return w

    def _make_asset_tab(self):
        w = QWidget()
        sub = QTabWidget()
        sub.addTab(InstallationFrame(), "설치자산관리")
        sub.addTab(InstallationSwFrame(), "SW상세관리")
        lay = QVBoxLayout(w)
        lay.setContentsMargins(0, 0, 0, 0)
        lay.addWidget(sub)
        return w

    def _make_mgmt_tab(self):
        w = CodeMasterFrame()
        return w


def run():
    app = QApplication([])
    win = App()
    win.show()
    app.exec()
