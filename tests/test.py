import kubectl
import settings
import test_operator
import util

from testflows.core import TestScenario, Name, When, Then, Given, And, main, run, Module, TE, args
from testflows.asserts import error

if main():
    with Module("main"):
        with Given(f"Clean namespace {settings.test_namespace}"):
            kubectl.delete_all_chi(settings.test_namespace)
            kubectl.delete_ns(settings.test_namespace, ok_to_fail=True)
            kubectl.create_ns(settings.test_namespace)

        # with Given(f"daisy-operator version {settings.operator_version} is installed"):
        #     if kubectl.get_count("pod", ns=settings.operator_namespace, label="-l app=clickhouse-operator") == 0:
        #         config = util.get_full_path('../deploy/operator/clickhouse-operator-install-template.yaml')
        #         kubectl.apply(
        #             ns=settings.operator_namespace,
        #             config=f"<(cat {config} | "
        #             f"OPERATOR_IMAGE=\"{settings.operator_docker_repo}:{settings.operator_version}\" "
        #             f"OPERATOR_NAMESPACE=\"{settings.operator_namespace}\" "
        #             f"METRICS_EXPORTER_IMAGE=\"{settings.metrics_exporter_docker_repo}:{settings.operator_version}\" "
        #             f"METRICS_EXPORTER_NAMESPACE=\"{settings.operator_namespace}\" "
        #             f"envsubst)",
        #             validate=False
        #         )
        #     test_operator.set_operator_version(settings.operator_version)
        #
        # with Given(f"Install ClickHouse template {settings.clickhouse_template}"):
        #     kubectl.apply(util.get_full_path(settings.clickhouse_template), settings.test_namespace)

        with Given(f"ClickHouse version {settings.clickhouse_version}"):
            pass

        # python3 tests/test.py --only operator*
        with Module("operator"):
            all_tests = [
                test_operator.test_001,
            ]
            run_tests = all_tests

            # placeholder for selective test running
            # run_tests = [test_008, (test_009, {"version_from": "0.9.10"})]
            # run_tests = [test_002]

            for t in run_tests:
                if callable(t):
                    run(test=t)
                else:
                    run(test=t[0], args=t[1])

