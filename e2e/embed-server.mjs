import { createServer } from 'node:http';
const server=createServer((req,res)=>{
 const src=new URL(req.url,'http://127.0.0.1:18382').searchParams.get('src') || '';
 const escaped=src.replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;');
 res.setHeader('Content-Type','text/html');
 res.end(`<title>Third-party blog</title><h1>Embedded tool</h1><iframe title="Shared tool" src="${escaped}" style="width:100%;height:600px;border:0"></iframe>`);
});
server.listen(18382,'127.0.0.1');
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>server.close());
