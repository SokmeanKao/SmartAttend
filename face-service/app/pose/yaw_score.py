import numpy as np

from app.errors import FacePipelineError

VALID_POSES = frozenset({"FRONT", "LEFT", "RIGHT"})


def yaw_score(face: np.ndarray) -> float:
    detection = np.asarray(face, dtype=np.float32).reshape(15)
    right_eye_x, left_eye_x = float(detection[4]), float(detection[6])
    nose_x = float(detection[8])
    right_mouth_x, left_mouth_x = float(detection[10]), float(detection[12])

    eye_midpoint = (right_eye_x + left_eye_x) / 2.0
    mouth_midpoint = (right_mouth_x + left_mouth_x) / 2.0
    facial_midline = (eye_midpoint + mouth_midpoint) / 2.0
    interocular_distance = abs(left_eye_x - right_eye_x)
    if interocular_distance <= 1e-6:
        raise FacePipelineError("FACE_QUALITY_TOO_LOW", "face landmarks are invalid")
    return (nose_x - facial_midline) / interocular_distance


def classify_yaw(score: float, *, front_max: float, side_min: float) -> str:
    if abs(score) <= front_max:
        return "FRONT"
    if score <= -side_min:
        return "LEFT"
    if score >= side_min:
        return "RIGHT"
    return "INVALID"


def validate_expected_pose(
    *,
    expected_pose: str,
    score: float,
    front_max: float,
    side_min: float,
) -> str:
    expected = expected_pose.upper()
    if expected not in VALID_POSES:
        raise FacePipelineError("INVALID_POSE", "expected_pose is invalid")
    label = classify_yaw(score, front_max=front_max, side_min=side_min)
    if label != expected:
        raise FacePipelineError(
            "INVALID_POSE",
            f"detected pose {label} does not match expected pose {expected}",
        )
    return label
