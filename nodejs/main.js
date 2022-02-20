var http = require('http');
var fs = require('fs');
// var data = "";

http.get("http://cos.fun.cloud.ibm.com/apps/app.js", res => {
  let data = "";
  res.on("data", d => { data += d });
  res.on("end", function () {
    console.log("data: " + data );
    fs.writeFile("/app/app.js", data, function() {
      var app = require('/app/app.js');
      
      const server = http.createServer((req, res) => {
        let data = '';
        req.on('data', chunk => {
          data += chunk;
        })
        req.on('end', () => {
  	      console.log("Req: " + req.method + " " + req.url);
          data = app.handler(data);
  	      res.write(data)
          res.end();
        });
      });
      
      console.log("Listening on port 8080");
      server.listen(8080);
    });
  });
});
