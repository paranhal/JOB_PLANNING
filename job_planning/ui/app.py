# -*- coding: utf-8 -*-
"""메인 윈도우·탭 구성 — 통합 처리 이력(접수대장)·고객·담당자·장비 (구현_탭_파일구조_설계.md §3-1)."""
import sys

from PySide6.QtWidgets import QApplication, QMainWindow, QTabWidget, QWidget, QVBoxLayout

from .frames.work_log import WorkLogFrame
from .frames.master import MasterFrame


def run():
    app = QApplication(sys.argv)
    app.setApplicationName("업무일지")

    window = QMainWindow()
    window.setWindowTitle("업무일지 — 통합 처리 이력·고객·담당자·장비")
    window.setMinimumSize(600, 500)
    window.resize(800, 640)

    central = QWidget()
    window.setCentralWidget(central)
    layout = QVBoxLayout(central)
    layout.setContentsMargins(5, 5, 5, 5)

    tabs = QTabWidget()
    work_log_frame = WorkLogFrame()
    master_frame = MasterFrame()
    tabs.addTab(work_log_frame, "처리 이력(접수대장)")
    tabs.addTab(master_frame, "고객·담당자·장비")
    layout.addWidget(tabs)

    work_log_frame.load_list()
    master_frame.load_list()

    window.show()
    sys.exit(app.exec())
