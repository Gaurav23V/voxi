from voxi_worker.protocol import WorkerRequest, WorkerResponse


def test_worker_request_from_dict() -> None:
    request = WorkerRequest.from_dict(
        {
            "id": "job-1",
            "op": "transcribe_and_clean",
            "audio_format": "pcm_s16le",
            "sample_rate_hz": 16000,
            "audio_b64": "Zm9v",
        }
    )

    assert request.id == "job-1"
    assert request.op == "transcribe_and_clean"
    assert request.audio_format == "pcm_s16le"
    assert request.sample_rate_hz == 16000
    assert request.audio_b64 == "Zm9v"


def test_worker_response_to_dict_omits_none_fields() -> None:
    response = WorkerResponse(id="job-1", ok=True, transcript="hello", cleaned="Hello.")

    payload = response.to_dict()

    assert payload == {
        "id": "job-1",
        "ok": True,
        "transcript": "hello",
        "cleaned": "Hello.",
    }
