"""The stand-ins and the `needs` marker logic; no cluster needed."""
import pytest

import e2e_config
from standins.s3_backend import fake as s3_backend


def test_unmet_standins_lists_those_not_fake():
    standins = e2e_config.Standins(iam="real", objectstore="off")
    assert e2e_config.unmet_standins(standins, ["iam", "objectstore", "s3_backend"]) == {
        "iam": "real", "objectstore": "off"}


def test_unmet_standins_empty_when_all_are_fake():
    assert e2e_config.unmet_standins(e2e_config.Standins(), ["iam", "s3_backend"]) == {}


def test_unmet_standins_rejects_a_name_that_is_not_a_stand_in():
    with pytest.raises(ValueError, match="wcpsvc"):
        e2e_config.unmet_standins(e2e_config.Standins(), ["wcpsvc"])


def test_bucket_listing_ignores_minio_metadata():
    assert s3_backend.parse(".minio.sys\nalpha\nbeta\n\n") == {"alpha", "beta"}
