import { createServer } from 'node:http';
const server=createServer((req,res)=>{
 const url=new URL(req.url,'http://127.0.0.1:18382');
 if(url.pathname==='/navigation-redirect'){
  res.writeHead(302,{Location:'/navigation-destination'});res.end();return;
 }
 if(url.pathname==='/navigation-destination'){
  res.setHeader('Content-Type','text/html');res.end('<h1>External destination reached</h1>');return;
 }
 const src=url.searchParams.get('src') || '';
 const escaped=src.replaceAll('&','&amp;').replaceAll('"','&quot;').replaceAll('<','&lt;');
 res.setHeader('Content-Type','text/html');
 res.end(`<title>Third-party blog</title><h1>Embedded tool</h1><iframe title="Shared tool" src="${escaped}" style="width:100%;height:600px;border:0"></iframe>`);
});
server.listen(18382,'127.0.0.1');
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>server.close());
