# 사진 슬롯 — 링크(경로/URL)만 저장, 표시 시 이미지 로드

from PySide6.QtWidgets import QWidget, QHBoxLayout, QVBoxLayout, QLabel, QLineEdit, QPushButton, QSizePolicy
from PySide6.QtCore import Qt, QUrl
from PySide6.QtGui import QPixmap
from PySide6.QtNetwork import QNetworkAccessManager, QNetworkRequest


# 최대 미리보기 크기
PREVIEW_SIZE = 120


def _load_pixmap_from_path(path: str) -> QPixmap | None:
    path = path.strip()
    if not path:
        return None
    if path.startswith("file://"):
        path = path[7:]
    pix = QPixmap(path)
    if pix.isNull():
        return None
    return pix.scaled(PREVIEW_SIZE, PREVIEW_SIZE, Qt.AspectRatioMode.KeepAspectRatio, Qt.TransformationMode.SmoothTransformation)


class PhotoSlotWidget(QWidget):
    """링크 1개 입력 + 미리보기. 저장은 링크 문자열만."""
    def __init__(self, parent=None, label: str = "사진"):
        super().__init__(parent)
        self._label_text = label
        self._network = QNetworkAccessManager(self)
        self._network.finished.connect(self._on_reply_finished)
        self._reply = None
        self._build_ui()

    def _build_ui(self):
        layout = QHBoxLayout(self)
        layout.setContentsMargins(0, 2, 0, 2)
        self._preview = QLabel()
        self._preview.setFixedSize(PREVIEW_SIZE, PREVIEW_SIZE)
        self._preview.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self._preview.setStyleSheet("background-color: #eee; border: 1px solid #ccc;")
        self._preview.setText("사진")
        self._preview.setSizePolicy(QSizePolicy.Policy.Fixed, QSizePolicy.Policy.Fixed)
        layout.addWidget(self._preview)

        right = QWidget()
        right_layout = QVBoxLayout(right)
        right_layout.setContentsMargins(4, 0, 0, 0)
        self._url_edit = QLineEdit()
        self._url_edit.setPlaceholderText("파일 경로 또는 이미지 URL")
        self._url_edit.setMinimumWidth(320)
        self._url_edit.editingFinished.connect(self._load_preview)
        right_layout.addWidget(self._url_edit)
        btn = QPushButton("미리보기 갱신")
        btn.clicked.connect(self._load_preview)
        right_layout.addWidget(btn)
        layout.addWidget(right, stretch=1)

    def get_url(self) -> str:
        return self._url_edit.text().strip()

    def set_url(self, url: str):
        self._url_edit.setText((url or "").strip())
        self._load_preview()

    def _load_preview(self):
        url_or_path = self.get_url()
        if not url_or_path:
            self._preview.clear()
            self._preview.setText("사진")
            return
        if url_or_path.startswith("http://") or url_or_path.startswith("https://"):
            req = QNetworkRequest(QUrl(url_or_path))
            self._reply = self._network.get(req)
        else:
            pix = _load_pixmap_from_path(url_or_path)
            if pix and not pix.isNull():
                self._preview.setPixmap(pix)
                self._preview.setText("")
            else:
                self._preview.clear()
                self._preview.setText("사진")

    def _on_reply_finished(self, reply):
        if reply != self._reply:
            return
        self._reply = None
        if reply.error():
            self._preview.clear()
            self._preview.setText("오류")
            reply.deleteLater()
            return
        data = reply.readAll()
        reply.deleteLater()
        pix = QPixmap()
        if pix.loadFromData(data) and not pix.isNull():
            pix = pix.scaled(PREVIEW_SIZE, PREVIEW_SIZE, Qt.AspectRatioMode.KeepAspectRatio, Qt.TransformationMode.SmoothTransformation)
            self._preview.setPixmap(pix)
            self._preview.setText("")
        else:
            self._preview.clear()
            self._preview.setText("사진")
