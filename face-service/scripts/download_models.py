#!/usr/bin/env python3
import argparse
import hashlib
import os
import tempfile
import urllib.request
from pathlib import Path

YUNET_MODEL = "face_detection_yunet_2023mar.onnx"
YUNET_SHA256 = "8F2383E4DD3CFBB4553EA8718107FC0423210DC964F9F4280604804ED2552FA4"
YUNET_URL = (
    "https://github.com/opencv/opencv_zoo/raw/main/models/"
    f"face_detection_yunet/{YUNET_MODEL}"
)

SFACE_MODEL = "face_recognition_sface_2021dec.onnx"
SFACE_SHA256 = "0BA9FBFA01B5270C96627C4EF784DA859931E02F04419C829E83484087C34E79"
SFACE_URL = (
    "https://github.com/opencv/opencv_zoo/raw/main/models/"
    f"face_recognition_sface/{SFACE_MODEL}"
)

MODEL_PINS = (
    (YUNET_MODEL, YUNET_URL, YUNET_SHA256),
    (SFACE_MODEL, SFACE_URL, SFACE_SHA256),
)


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as model:
        for chunk in iter(lambda: model.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def verify_sha256(path: Path, expected_sha256: str) -> None:
    actual = file_sha256(path)
    expected = expected_sha256.upper()
    if actual != expected:
        raise ValueError(
            f"SHA256 mismatch for {path.name}: expected {expected}, got {actual}"
        )


def download_model(url: str, destination: Path, expected_sha256: str) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.exists():
        verify_sha256(destination, expected_sha256)
        return

    file_descriptor, temporary_name = tempfile.mkstemp(
        prefix=f"{destination.name}.", dir=destination.parent
    )
    os.close(file_descriptor)
    temporary_path = Path(temporary_name)
    try:
        with urllib.request.urlopen(url, timeout=120) as response:
            with temporary_path.open("wb") as output:
                while chunk := response.read(1024 * 1024):
                    output.write(chunk)
        verify_sha256(temporary_path, expected_sha256)
        temporary_path.replace(destination)
    finally:
        temporary_path.unlink(missing_ok=True)


def verify_models(model_dir: Path) -> None:
    for filename, _, expected_sha256 in MODEL_PINS:
        path = model_dir / filename
        if not path.is_file():
            raise FileNotFoundError(f"required model is missing: {path}")
        verify_sha256(path, expected_sha256)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-dir", type=Path, default=Path("models"))
    parser.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()

    if args.verify_only:
        verify_models(args.model_dir)
        return
    for filename, url, expected_sha256 in MODEL_PINS:
        download_model(url, args.model_dir / filename, expected_sha256)


if __name__ == "__main__":
    main()
