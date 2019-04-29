# dronio
General purpose fly&vtx driver & (future) mobile app for visuo xs809hw (and some others) writen in go


Alert: **Some heavy reverse engineering involved!**

This is mostly based on capturing data packets from official android app and trying to figure out how does things work.
Some of the captures can be found in the `analysis` directory, together with my notes.

Main package does basicaly nothing now - don't even bother building it.
But sub-packages `github.com/drahoslove/dronio/fly` and  `github.com/drahoslove/dronio/vtx` can be used independently to control flight and/or video transmitting respectively - those are kind of working.

Package `fly` is compatible with `gobot.io`'s `gobot.Driver` interface and I might create PR one day. 

