import json
import time

import pytest
from langevals import expect
from langevals_langevals.llm_boolean import (
    CustomLLMBooleanEvaluator,
    CustomLLMBooleanSettings,
)
from litellm import Message, acompletion
from mcp import ClientSession

from conftest import models
from utils import (
    get_converted_tools,
    llm_tool_call_sequence,
)

pytestmark = pytest.mark.anyio


@pytest.mark.parametrize("model", models)
@pytest.mark.flaky(max_runs=3)
async def test_tempo_search_traces(model: str, mcp_client: ClientSession):
    # Wait for some traces to be generated
    time.sleep(5)
    
    tools = await get_converted_tools(mcp_client)
    prompt = "Can you search for recent traces in Tempo and show me what services are generating traces? Please limit to 10 traces."

    messages = [
        Message(role="system", content="You are a helpful assistant."),
        Message(role="user", content=prompt),
    ]

    # 1. Search for traces
    messages = await llm_tool_call_sequence(
        model,
        messages,
        tools,
        mcp_client,
        "search_tempo_traces",
        {"datasourceUid": "tempo", "limit": 10}
    )

    # 2. Final LLM response
    response = await acompletion(model=model, messages=messages, tools=tools)
    content = response.choices[0].message.content
    trace_search_checker = CustomLLMBooleanEvaluator(
        settings=CustomLLMBooleanSettings(
            prompt=(
                "Does the response contain specific information about traces found in Tempo? "
                "It should mention trace IDs, service names, durations, or other trace metadata."
            ),
        )
    )
    expect(input=prompt, output=content).to_pass(trace_search_checker)


@pytest.mark.parametrize("model", models)
@pytest.mark.flaky(max_runs=3)
async def test_tempo_tag_exploration(model: str, mcp_client: ClientSession):
    tools = await get_converted_tools(mcp_client)
    prompt = "What tags are available in Tempo for searching traces? Can you also show me some values for the service.name tag?"

    messages = [
        Message(role="system", content="You are a helpful assistant."),
        Message(role="user", content=prompt),
    ]

    # 1. List tag names
    messages = await llm_tool_call_sequence(
        model,
        messages,
        tools,
        mcp_client,
        "list_tempo_tag_names",
        {"datasourceUid": "tempo"}
    )

    # 2. List tag values for service.name
    messages = await llm_tool_call_sequence(
        model,
        messages,
        tools,
        mcp_client,
        "list_tempo_tag_values",
        {"datasourceUid": "tempo", "tagName": "service.name"}
    )

    # 3. Final LLM response
    response = await acompletion(model=model, messages=messages, tools=tools)
    content = response.choices[0].message.content
    tag_exploration_checker = CustomLLMBooleanEvaluator(
        settings=CustomLLMBooleanSettings(
            prompt=(
                "Does the response contain information about available tags in Tempo "
                "and specific values for the service.name tag? It should list various "
                "tag names and show actual service names found in the traces."
            ),
        )
    )
    expect(input=prompt, output=content).to_pass(tag_exploration_checker)


@pytest.mark.parametrize("model", models)
@pytest.mark.flaky(max_runs=3)
async def test_tempo_trace_analysis(model: str, mcp_client: ClientSession):
    # Wait for some traces to be generated
    time.sleep(5)
    
    tools = await get_converted_tools(mcp_client)
    prompt = "Can you find a trace with a duration longer than 10ms and analyze its structure?"

    messages = [
        Message(role="system", content="You are a helpful assistant."),
        Message(role="user", content=prompt),
    ]

    # 1. Search for traces with duration filter
    messages = await llm_tool_call_sequence(
        model,
        messages,
        tools,
        mcp_client,
        "search_tempo_traces",
        {"datasourceUid": "tempo", "minDuration": "10ms", "limit": 1}
    )

    # Parse the trace ID from the response
    search_response = messages[-1].content
    search_data = json.loads(search_response)
    
    if search_data.get("traces") and len(search_data["traces"]) > 0:
        trace_id = search_data["traces"][0]["traceID"]
        
        # 2. Get the full trace
        messages = await llm_tool_call_sequence(
            model,
            messages,
            tools,
            mcp_client,
            "get_tempo_trace",
            {"datasourceUid": "tempo", "traceId": trace_id}
        )

    # 3. Final LLM response
    response = await acompletion(model=model, messages=messages, tools=tools)
    content = response.choices[0].message.content
    trace_analysis_checker = CustomLLMBooleanEvaluator(
        settings=CustomLLMBooleanSettings(
            prompt=(
                "Does the response contain an analysis of a specific trace? "
                "It should mention the trace ID, duration, service names involved, "
                "and potentially information about spans within the trace."
            ),
        )
    )
    expect(input=prompt, output=content).to_pass(trace_analysis_checker)