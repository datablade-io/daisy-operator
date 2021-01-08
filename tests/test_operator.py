import time

import clickhouse
import kubectl
import settings
import util
import manifest

from testflows.core import TestScenario, Name, When, Then, Given, And, main, run, Module, TE
from testflows.asserts import error


@TestScenario
@Name("test_001. 1 node")
def test_001():
    kubectl.create_and_check(
        config="configs/test-001.yaml",
        check={
            "object_counts": {
                "statefulset": 1,
                "pod": 1,
                "service": 2,
            }
            #, "configmaps": 0,
        }
    )

