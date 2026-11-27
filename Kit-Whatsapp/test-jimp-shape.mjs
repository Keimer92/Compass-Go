const ns = await import('jimp');
console.log('typeof ns.Jimp =', typeof ns.Jimp);
console.log('typeof ns.default =', typeof ns.default);
console.log('ResizeStrategy =', typeof ns.ResizeStrategy);
console.log('Jimp.read =', typeof ns.Jimp?.read);
