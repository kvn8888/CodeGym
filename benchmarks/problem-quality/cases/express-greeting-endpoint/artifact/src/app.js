const express = require('express');

const app = express();

app.get('/greeting', (req, res) => {
  const name = req.query.name;

  if (!name || !String(name).trim()) {
    return res.status(400).json({ error: 'name is required' });
  }

  return res.status(200).json({ message: `Hello, ${name}!` });
});

const port = process.env.PORT || 3000;
app.listen(port, () => {
  console.log(`Server listening on port ${port}`);
});

module.exports = app;
