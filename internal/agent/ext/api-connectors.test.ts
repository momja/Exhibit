import { test } from "node:test";
import assert from "node:assert/strict";
import { registerAPIConnectors } from "./api-connectors.ts";

const spec = {name: "Weather", origin: "https://api.weather.com", auth: "api-key", reason: "Show weather"};
function toolFor(proposal) {
 let tool;
 registerAPIConnectors({registerTool(t) {tool=t;}}, async (method,path,body) => {
  assert.equal(method,"POST"); assert.equal(path,"/api/artifacts/app/agent-api-proposals");
  assert.deepEqual(body,spec); return {proposal,secret:"future-field-never-in-context"};
 }, () => "app");
 return tool;
}
test("private connector tool returns only safe outcome and handles decline, abort and removed connections", async () => {
 for (const status of ["approved","denied","disconnected"]) {
  const tool=toolFor({state:"pending",id:"proposal-1"});
  const result=await tool.execute("call-1",spec,new AbortController().signal,null,{ui:{input:async (title,payload) => {
   assert.equal(title,"exhibit:api-connector:v1");assert.deepEqual(JSON.parse(payload),{toolCallId:"call-1",proposalId:"proposal-1"});
   return JSON.stringify({status,secret:"reply-field-never-in-context"});
  }}});
  assert.equal(result.details.status,status);
  assert.doesNotMatch(JSON.stringify(result),/never-in-context/);
  if (status==="denied") assert.match(result.content[0].text,/Do not repeat this request/);
  if (status==="disconnected") assert.match(result.content[0].text,/connection was removed/);
 }
 const abort=new AbortController();abort.abort();
 const result=await toolFor({state:"pending",id:"proposal-1"}).execute("call-1",spec,abort.signal,null,{ui:{input(){assert.fail("aborted request cannot ask");}}});
 assert.equal(result.details.status,"denied");
});
test("sticky denial and existing account reuse do not ask for another key",async()=>{
 for (const state of ["approved","denied","disconnected"]) {
  const result=await toolFor({state}).execute("call-1",spec,new AbortController().signal,null,{ui:{input(){assert.fail("terminal proposal cannot ask");}}});
  assert.equal(result.details.status,state);
 }
});
