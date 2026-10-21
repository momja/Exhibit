import { test } from 'node:test';
import assert from 'node:assert/strict';
import { installSourceContext, SOURCE_READ_ENTRY } from './source-context.ts';
const model = {api: 'openai-completions', provider: 'test', id: 'm'};
function rig({saved = [], fail = false, revision = 'r1'} = {}) {
  const hooks = new Map();
  const entries = [...saved];
  const notices = [];
  const adopted = [];
  let reads = 0;
  let currentRevision = revision;
  let unavailable = fail;
  const pi = {on: (type, fn) => hooks.set(type, fn), appendEntry: (customType, data) => entries.push({type: 'custom', customType, data})};
  const ctx = {model, sessionManager: {getBranch: () => entries}, ui: {notify: message => notices.push(message)}};
  installSourceContext(pi, {
    artifactId: () => 'a',
    read: async () => {
      reads++;
      if (unavailable) throw new Error('offline');
      return {artifactId: 'a', sourceRevision: currentRevision, artifact: {body: 'SOURCE-' + currentRevision},
        result: {content: [{type: 'text', text: 'SOURCE-' + currentRevision}], details: {exhibit: 'artifact_read'}}};
    },
    revision: async () => { if (unavailable) throw new Error('offline'); return currentRevision; },
    adopt: snapshot => adopted.push(snapshot),
  });
  return {entries, notices, adopted, get reads() { return reads; }, set revision(value) {currentRevision = value;}, set unavailable(value) {unavailable = value;},
    emit: (type, event = {}) => hooks.get(type)(event, ctx),
    user(id, text = 'change it') {const message = {role: 'user', content: [{type: 'text', text}], timestamp: entries.length + 10}; entries.push({type: 'message', id, message}); return message;},
  };
}
function assertPair(messages, source) {
  const [assistant, tool] = messages.slice(-2);
  assert.equal(assistant.role, 'assistant');
  assert.equal(assistant.content[0].name, 'get_artifact');
  assert.equal(tool.role, 'toolResult');
  assert.equal(assistant.content[0].id, tool.toolCallId);
  assert.equal(tool.content[0].text, source);
  for (const message of messages) if (message.role !== 'toolResult') assert.ok(!JSON.stringify(message).includes('SOURCE-'));
}

test('startup reads source with no messages; accepted context persists data and projects a matching pair', async () => {
  const r = rig();
  await r.emit('session_start');
  assert.equal(r.reads, 1);
  assert.equal(r.entries.length, 0);
  const user = r.user('u1');
  const result = await r.emit('context', {messages: [user]});
  assertPair(result.messages, 'SOURCE-r1');
  assert.equal(r.reads, 1);
  assert.equal(r.entries[1].customType, SOURCE_READ_ENTRY);
  assert.equal(r.adopted[0].sourceRevision, 'r1');
  const again = await r.emit('context', {messages: [user]});
  assert.deepEqual(again, result);
  assert.equal(r.notices.length, 1);
});

test('source edited between prepare and Send refreshes and adopts the replacement', async () => {
  const r = rig(); await r.emit('session_start'); r.revision = 'r2';
  const user = r.user('u1');
  assertPair((await r.emit('context', {messages: [user]})).messages, 'SOURCE-r2');
  assert.equal(r.reads, 2);
  assert.equal(r.adopted[0].artifact.body, 'SOURCE-r2');
});

test('missing OCC revision rereads at Send; failed validation falls back without adopting stale source', async () => {
  const r = rig({revision: undefined}); await r.emit('session_start');
  r.revision = undefined;
  const user = r.user('u1');
  await r.emit('context', {messages: [user]}); assert.equal(r.reads, 2);
  const failing = rig(); await failing.emit('session_start'); failing.unavailable = true;
  const current = failing.user('u');
  assert.deepEqual((await failing.emit('context', {messages: [current]})).messages, [current]);
  assert.equal(failing.adopted.length, 0);
});

test('resume replays persisted pair and refreshes current prompt; old sessions need no custom entries', async () => {
  const r = rig(); await r.emit('session_start'); const oldUser = r.user('u1');
  await r.emit('context', {messages: [oldUser]});
  const resumed = rig({saved: r.entries, revision: 'r2'}); await resumed.emit('session_start');
  const nextUser = resumed.user('u2');
  const messages = (await resumed.emit('context', {messages: [oldUser, nextUser]})).messages;
  assertPair(messages.slice(0, 3), 'SOURCE-r1'); assertPair(messages, 'SOURCE-r2');
  const legacy = rig({saved: [{type: 'message', id: 'old', message: oldUser}]});
  await legacy.emit('session_start'); const current = legacy.user('new');
  assertPair((await legacy.emit('context', {messages: [oldUser, current]})).messages, 'SOURCE-r1');
});

test('compaction drops old projections and refreshes current source for overflow retry', async () => {
  const r = rig(); await r.emit('session_start'); const old = r.user('u1');
  await r.emit('context', {messages: [old]}); r.revision = 'r2';
  await r.emit('session_compact');
  const summary = {role: 'user', content: [{type: 'text', text: 'summary'}], timestamp: 100};
  const messages = (await r.emit('context', {messages: [summary]})).messages;
  assertPair(messages, 'SOURCE-r2'); assert.equal(messages.length, 3);
  assert.ok(!JSON.stringify(messages).includes('SOURCE-r1'));
});

test('a blocked input never reaches context, never adopts or persists a prepared read', async () => {
  const r = rig(); await r.emit('session_start');
  assert.equal(r.adopted.length, 0); assert.equal(r.entries.length, 0); assert.equal(r.notices.length, 0);
});

test('an own save advances the known snapshot without a new source read or projection', async () => {
 const hooks = new Map(); const entries = []; let reads = 0; let current = null; let revision = 'r1';
 const snapshot = value => ({artifactId:'a', sourceRevision:value, artifact:{body:'SOURCE-'+value}, result:{content:[{type:'text',text:'SOURCE-'+value}],details:{}}});
 const pi = {on:(type, fn)=>hooks.set(type,fn), appendEntry:(customType,data)=>entries.push({type:'custom',customType,data})};
 const ctx = {model,sessionManager:{getBranch:()=>entries},ui:{notify(){}}};
 installSourceContext(pi,{artifactId:()=> 'a', read:async()=>{reads++;return snapshot(revision);},revision:async()=>revision,
  current:()=>current,adopt:value=>{current=value;}});
 await hooks.get('session_start')({},ctx);
 const user = (id,timestamp) => {const message={role:'user',content:[{type:'text',text:id}],timestamp};entries.push({type:'message',id,message});return message;};
 const firstUser = user('u1',10);
 await hooks.get('context')({messages:[firstUser]},ctx);
 revision='r2';
 // Even an in-place cache update must not rewrite historical source evidence.
 current.sourceRevision='r2';current.artifact.body='SOURCE-r2';current.result.content[0].text='SOURCE-r2';
 const secondUser = user('u2',20);
 const result=await hooks.get('context')({messages:[firstUser, {role:'assistant',content:[{type:'toolCall',id:'write',name:'edit_artifact',arguments:{edits:[]}}]},
  {role:'toolResult',toolName:'edit_artifact',toolCallId:'write',content:[{type:'text',text:'Updated artifact'}]}, secondUser]},ctx);
 assert.equal(reads,1);
 assert.equal(result.messages.filter(message=>message.role==='toolResult' && message.toolName==='get_artifact').length,1);
 assert.ok(!JSON.stringify(result.messages).includes('SOURCE-r2'), 'own updated source is known through the edit, not reinjected');
 assert.equal(entries.filter(entry=>entry.customType===SOURCE_READ_ENTRY).length,1);
 assert.equal(current.sourceRevision,'r2');
});


test('unchanged successive user turns reuse the retained base without another read or pair', async () => {
 const r=rig();await r.emit('session_start');const first=r.user('u1');
 await r.emit('context',{messages:[first]});const second=r.user('u2');
 const result=await r.emit('context',{messages:[first,second]});
 assert.equal(result.messages.length,4);assert.equal(r.reads,1);
 assert.equal(r.entries.filter(entry=>entry.customType===SOURCE_READ_ENTRY).length,1);
 assert.equal(r.notices.length,1);
});

test('if the retained base disappears, the known snapshot is projected again', async () => {
 const r=rig();await r.emit('session_start');const first=r.user('u1');
 await r.emit('context',{messages:[first]});const second=r.user('u2');
 assertPair((await r.emit('context',{messages:[second]})).messages,'SOURCE-r1');
 assert.equal(r.entries.filter(entry=>entry.customType===SOURCE_READ_ENTRY).length,2);
});

test('an invalidated edit cache forces a fresh read even if a retained base has the same token', async () => {
 const hooks=new Map();const entries=[];let current=null;let readCount=0;
 const snapshot=()=>({artifactId:'a',sourceRevision:'r1',artifact:{body:'SOURCE-r1'},result:{content:[{type:'text',text:'SOURCE-r1'}],details:{}}});
 const pi={on:(type,fn)=>hooks.set(type,fn),appendEntry:(customType,data)=>entries.push({type:'custom',customType,data})};
 const ctx={model,sessionManager:{getBranch:()=>entries},ui:{notify(){}}};
 installSourceContext(pi,{artifactId:()=> 'a',read:async()=>{readCount++;return snapshot();},revision:async()=> 'r1',
  current:()=>current,adopt:value=>{current=value;}});
 await hooks.get('session_start')({},ctx);
 const first={role:'user',content:[{type:'text',text:'first'}],timestamp:10};entries.push({type:'message',id:'u1',message:first});
 await hooks.get('context')({messages:[first]},ctx);
 current=null; // conflict/network uncertainty invalidated the edit cache
 const second={role:'user',content:[{type:'text',text:'second'}],timestamp:20};entries.push({type:'message',id:'u2',message:second});
 const result=await hooks.get('context')({messages:[first,second]},ctx);
 assert.equal(readCount,2);assertPair(result.messages,'SOURCE-r1');
 assert.equal(entries.filter(entry=>entry.customType===SOURCE_READ_ENTRY).length,2);
});

test('compaction that retains the source base does not inject a second full-source pair', async () => {
 const r=rig();await r.emit('session_start');const first=r.user('u1');
 await r.emit('context',{messages:[first]});await r.emit('session_compact');
 const result=await r.emit('context',{messages:[first]});
 assertPair(result.messages,'SOURCE-r1');assert.equal(result.messages.length,3);
 assert.equal(r.reads,1);assert.equal(r.entries.filter(entry=>entry.customType===SOURCE_READ_ENTRY).length,1);
});

test('split-turn compaction never appends obsolete preparation after retained reads and edits', async () => {
 const hooks = new Map(); const entries = []; let known = null; let revision = 'r1'; let reads = 0;
 const snapshot = value => ({artifactId:'a',sourceRevision:value,artifact:{body:'SOURCE-'+value},
  result:{content:[{type:'text',text:'SOURCE-'+value}],details:{}}});
 const pi = {on:(type,fn)=>hooks.set(type,fn),appendEntry:(customType,data)=>entries.push({type:'custom',customType,data})};
 const ctx = {model,sessionManager:{getBranch:()=>entries},ui:{notify(){}}};
 installSourceContext(pi,{artifactId:()=> 'a',read:async()=>{reads++;return snapshot(revision);},revision:async()=>revision,
  current:()=>known,adopt:value=>{known=value;}});
 await hooks.get('session_start')({},ctx);
 const user={role:'user',content:[{type:'text',text:'edit'}],timestamp:1};entries.push({type:'message',id:'u1',message:user});
 await hooks.get('context')({messages:[user]},ctx);
 revision='r2';known=snapshot(revision);
 const retained=[{role:'user',content:[{type:'text',text:'compaction summary'}],timestamp:2},
  {role:'toolResult',toolName:'get_artifact',toolCallId:'real-read',content:[{type:'text',text:'SOURCE-r1'}]},
  {role:'assistant',content:[{type:'toolCall',id:'edit',name:'edit_artifact',arguments:{edits:[{oldText:'SOURCE-r1',newText:'SOURCE-r2'}]}}]},
  {role:'toolResult',toolName:'edit_artifact',toolCallId:'edit',content:[{type:'text',text:'Updated artifact'}]}];
 await hooks.get('session_compact')({},ctx);
 assert.deepEqual((await hooks.get('context')({messages:retained},ctx)).messages,retained);
 assert.deepEqual((await hooks.get('context')({messages:retained},ctx)).messages,retained,'later provider requests cannot revive the obsolete fallback');
 assert.equal(reads,1);assert.equal(known.sourceRevision,'r2');
 revision='r3';await hooks.get('session_compact')({},ctx);
 assertPair((await hooks.get('context')({messages:retained},ctx)).messages.slice(-2),'SOURCE-r3');
 assert.equal(reads,2,'external changes still project the latest validated source');
});
