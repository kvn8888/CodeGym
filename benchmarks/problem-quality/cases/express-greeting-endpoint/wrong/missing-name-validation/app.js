const express = require('express');

const app = express();

app.get('/greeting', (req, res) => {
  const name = req.query.name;
  return res.status(200).json({ message: `Hello, ${name || ''}!` });
});

const port = process.env.PORT || 3000;
app.listen(port, () => {
  console.log(`Server listening on port ${port}`);
});

module.exports = app;
